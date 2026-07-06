package protocoldebug

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/mahiro424/cbs/internal/protocol"
)

// Run 执行 protocol-debug CLI，返回进程退出码。
func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "用法：protocol-debug inspect|compare [flags]")
		return 2
	}
	switch args[0] {
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "compare":
		return runCompare(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "未知子命令：%s\n", args[0])
		return 2
	}
}

func runInspect(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	frameHex := fs.String("hex", "", "要 inspect 的业务帧十六进制")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	inspection, err := protocol.InspectBusinessPacketHex(*frameHex)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return writeJSON(stdout, stderr, inspection)
}

func runCompare(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	expectedHex := fs.String("expected", "", "期望业务帧十六进制")
	actualHex := fs.String("actual", "", "实际业务帧十六进制")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	comparison, err := protocol.CompareBusinessPacketHex(*expectedHex, *actualHex)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return writeJSON(stdout, stderr, comparison)
}

func writeJSON(stdout io.Writer, stderr io.Writer, value any) int {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return 1
	}
	_, _ = stdout.Write(append(payload, '\n'))
	return 0
}
