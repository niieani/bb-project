package app

import "os/exec"

func defaultRunCommand(name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	output, err := command.CombinedOutput()
	return string(output), err
}
