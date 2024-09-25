package task

import (
	"fmt"
	"strings"

	"github.com/ndsky1003/buffer"
)

type ILogger interface {
	Infof(format string, v ...any)
	Errf(format string, v ...any)
	Info(v ...any)
	Err(v ...any)
}

type default_logger struct{}

func (this *default_logger) Infof(format string, v ...any) {
	if !strings.HasSuffix(format, "\n") {
		buf := buffer.Get()
		buf.WriteString(format)
		buf.WriteByte('\n')
		format = buf.String()
		buf.Release()
	}
	fmt.Printf(format, v...)
}

func (this *default_logger) Errf(format string, v ...any) {
	this.Infof(format, v...)
}

func (this *default_logger) Info(v ...any) {
	fmt.Println(v...)
}

func (this *default_logger) Err(v ...any) {
	this.Info(v...)
}

var logger ILogger = &default_logger{}
