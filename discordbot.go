package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

//function to start discord bot
func StartDiscordBot() {
	cmd := "node"
	botpath := "discord/index.js"
	process := exec.Command(cmd, botpath)
	stdin, err := process.StdinPipe()
	if err != nil {
		fmt.Println(err)
	}
	defer stdin.Close()
	buf := new(bytes.Buffer)
	process.Stdout = buf
	process.Stderr = os.Stderr

	if err = process.Start(); err != nil {
		fmt.Println("An error occured: ", err)
	}

	process.Wait()
	fmt.Println("Generated string:", buf)
}
