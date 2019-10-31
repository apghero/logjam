package logjam

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	reset        = "\033[0m"
	yellow       = "\033[1;33m"
	red          = "\033[1;31m"
	announcement = "\033[1;32m"

	cold int = iota
	coolingDown
	heatingUp
	onFire
)

func blazing(txt []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(red)
	buf.Write(txt)
	buf.WriteString(reset)
	return buf.Bytes()
}

func heating(txt []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(yellow)
	buf.Write(txt)
	buf.WriteString(reset)
	return buf.Bytes()
}

func announce(a string) []byte {
	var buf bytes.Buffer
	buf.WriteString(announcement)
	buf.WriteString(a)
	buf.WriteString(reset)
	return buf.Bytes()
}

func fire(txt []byte) []byte {
	var buf bytes.Buffer
	for i, c := range txt {
		if uint64(i)&1 == 0 {
			buf.WriteString(red)
		} else {
			buf.WriteString(yellow)
		}
		buf.WriteByte(c)
	}
	buf.WriteString(reset)
	return buf.Bytes()
}

type Logger struct {
	mu     sync.Mutex // ensures atomic writes; protects the following fields
	prefix string     // prefix to write at beginning of each line
	term   bool
	out    io.Writer // destination for output
	buf    []byte

	state          int
	period         int64
	firePeriod     int64
	rate           int
	rateHeatingUp  int
	rateOnFire     int
	periodsBlazing int64
	announce       string
	heat           func([]byte) []byte
}

func New(out io.Writer, prefix string) *Logger {
	return &Logger{
		out:            out,
		prefix:         prefix,
		term:           true,
		state:          cold,
		rateHeatingUp:  10,
		rateOnFire:     20,
		periodsBlazing: 5,
	}
}

func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
}

func (l *Logger) SetHeatingUp(hup int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rateHeatingUp = hup
}

func (l *Logger) SetOnFire(of int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rateOnFire = of
}

func (l *Logger) SetBlazing(b int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.periodsBlazing = int64(b)
}

func (l *Logger) updateHeat(now time.Time) {
	l.rate += 1
	period := now.Unix()
	if l.period == period {
		return
	}

	// reset
	l.period = period

	switch l.state {
	case cold:
		l.heat = nil
		if l.rate > l.rateHeatingUp {
			l.announce = "It's heating up!!! "
			l.state = heatingUp
			l.heat = heating
		}
		break

	case coolingDown:
		if l.rate > l.rateHeatingUp {
			l.heat = heating
			l.state = heatingUp
		} else if l.rate < l.rateHeatingUp {
			l.heat = nil
			l.state = cold
		}
		break

	case heatingUp:
		l.heat = heating
		if l.rate > l.rateOnFire {
			l.announce = "It's on fire!!! "
			l.state = onFire
			l.firePeriod = period
			l.heat = fire
		}
		break

	case onFire:
		l.heat = fire
		// maybe we cooled off.
		if l.rate < l.rateOnFire {
			l.state = coolingDown
			l.heat = heating
			break
		}
		if l.firePeriod+l.periodsBlazing < period {
			l.announce = "Boomshakalaka!!! "
			l.heat = blazing
		}
	}
	l.rate = 0
}

func (l *Logger) Output(s string) error {
	now := time.Now() // get this early.
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = l.buf[:0]
	l.updateHeat(now)

	if l.announce != "" {
		l.buf = append(l.buf, announce(l.announce)...)
		l.announce = ""
	}

	nl := len(s) == 0 || s[len(s)-1] != '\n'

	if l.heat != nil && l.term {
		l.buf = append(l.buf, l.heat([]byte(s))...)
	} else {
		l.buf = append(l.buf, s...)
	}

	if nl {
		l.buf = append(l.buf, '\n')
	}
	_, err := l.out.Write(l.buf)

	return err
}

func (l *Logger) Printf(format string, v ...interface{}) {
	l.Output(fmt.Sprintf(format, v...))
}

func (l *Logger) Print(v ...interface{}) {
	l.Output(fmt.Sprint(v...))
}

func (l *Logger) Println(v ...interface{}) {
	l.Output(fmt.Sprintln(v...))
}

func (l *Logger) Fatal(v ...interface{}) {
	l.Output(fmt.Sprint(v...))
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.Output(fmt.Sprintf(format, v...))
	os.Exit(1)
}

func (l *Logger) Fatalln(v ...interface{}) {
	l.Output(fmt.Sprintln(v...))
	os.Exit(1)
}

func (l *Logger) Panic(v ...interface{}) {
	s := fmt.Sprint(v...)
	l.Output(s)
	panic(s)
}

func (l *Logger) Panicf(format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	l.Output(s)
	panic(s)
}

func (l *Logger) Panicln(v ...interface{}) {
	s := fmt.Sprintln(v...)
	l.Output(s)
	panic(s)
}

// Prefix returns the output prefix for the logger.
func (l *Logger) Prefix() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.prefix
}

// SetPrefix sets the output prefix for the logger.
func (l *Logger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
}

// Writer returns the output destination for the logger.
func (l *Logger) Writer() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.out
}
