package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func syncDir(source, target, opmode, sshkey string) error {
	args := []string{}

	if strings.Contains(opmode, "archive") {
		args = append(args, "--archive")
	}
	if strings.Contains(opmode, "compress") {
		args = append(args, "--compress")
	}
	if strings.Contains(opmode, "delete") {
		args = append(args, "--delete")
		args = append(args, "--recursive")
	} else if strings.Contains(opmode, "recursive") {
		args = append(args, "--recursive")
	}
	if sshkey != "" {
		args = append(args, "-e")
		args = append(args, fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=quiet -i %v", sshkey))
	}

	args = append(args, fmt.Sprintf("%v/", source))
	args = append(args, target)

	if out, err := exec.Command("rsync", args...).CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}

	return nil
}
