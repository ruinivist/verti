package output

import (
	"fmt"
	"strings"
)

const (
	italic     = "\x1b[3m"
	orange     = "\x1b[38;2;217;119;6m"
	white      = "\x1b[38;2;255;255;255m"
	orangeBg   = "\x1b[48;2;217;119;6m"
	reset      = "\x1b[0m"
	prefix     = orangeBg + white + " " + italic + "Verti" + reset + orangeBg + white + " " + reset + " "
)

func Prefix() string {
	return prefix
}

func Format(msg string) string {
	body := strings.TrimRight(msg, "\n")
	suffix := msg[len(body):]
	return Prefix() + orange + body + reset + suffix
}

func Println(msg string) {
	fmt.Println(Format(msg))
}

func Printf(format string, args ...any) {
	fmt.Print(Format(fmt.Sprintf(format, args...)))
}
