package cli

import "testing"

func TestRootRegistersDoctorCommand(t *testing.T) {
	root := NewRootCommand()

	doctorCmd, _, err := root.Find([]string{"doctor"})
	if err != nil {
		t.Fatalf("find doctor command: %v", err)
	}
	if doctorCmd == nil || doctorCmd.Name() != "doctor" {
		t.Fatalf("expected doctor command, got %#v", doctorCmd)
	}

	tracerCmd, _, err := root.Find([]string{"doctor", "tracer"})
	if err != nil {
		t.Fatalf("find doctor tracer command: %v", err)
	}
	if tracerCmd == nil || tracerCmd.Name() != "tracer" {
		t.Fatalf("expected tracer command, got %#v", tracerCmd)
	}
}
