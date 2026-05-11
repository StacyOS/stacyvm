package providers

import (
	"reflect"
	"testing"
)

func TestBuildExecCommandShellMode(t *testing.T) {
	got, err := buildExecCommand(ExecOptions{
		Command: "echo",
		Args:    []string{"hello world", "it's ok"},
	})
	if err != nil {
		t.Fatalf("build exec command: %v", err)
	}

	want := []string{"sh", "-c", "echo 'hello world' 'it'\"'\"'s ok'"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
}

func TestBuildExecCommandArgvMode(t *testing.T) {
	got, err := buildExecCommand(ExecOptions{
		Mode:    ExecModeArgv,
		Command: "printf",
		Args:    []string{"%s", "$HOME && echo injected"},
	})
	if err != nil {
		t.Fatalf("build exec command: %v", err)
	}

	want := []string{"printf", "%s", "$HOME && echo injected"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
}

func TestBuildExecCommandRejectsUnsupportedMode(t *testing.T) {
	if _, err := buildExecCommand(ExecOptions{Mode: "raw", Command: "echo"}); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}
