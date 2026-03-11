package output

import "fmt"

// orange bg with black text
const prefix = "\x1b[48;2;217;119;6m\x1b[38;2;255;255;255m Verti \x1b[0m "

func Prefix() string {
	return prefix
}

func Println(msg string) {
	fmt.Println(Prefix() + msg)
}

func Printf(format string, args ...any) {
	fmt.Print(Prefix() + fmt.Sprintf(format, args...))
}
