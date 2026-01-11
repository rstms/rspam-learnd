package sample

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

const Version = "0.0.3"

type Sample struct {
	Username string
	Class    string
	Domains  []string
	Message  *[]byte
	verbose  bool
}

func NewSample(class, username string, domains []string, message *[]byte) *Sample {
	s := Sample{
		Class:    class,
		Username: username,
		Domains:  domains,
		Message:  message,
		verbose:  ViperGetBool("verbose"),
	}
	return &s
}

func (s *Sample) Submit() {
	if s.verbose {
		log.Printf("Submitting %s %s domains=%v\n", s.Username, s.Class, s.Domains)
	}
	for _, domain := range s.Domains {
		args := []string{"-d", fmt.Sprintf("%s@%s", s.Username, domain)}
		if s.verbose {
			args = append(args, "-v")
		}
		args = append(args, "learn_"+s.Class)
		if s.verbose {
			log.Printf("cmd=rspamc %s\n", strings.Join(args, " "))
		}
		cmd := exec.Command("rspamc", args...)
		cmd.Stdin = bytes.NewBuffer(*s.Message)
		var oBuf bytes.Buffer
		var eBuf bytes.Buffer
		cmd.Stdout = &oBuf
		cmd.Stderr = &eBuf
		exitCode := -1
		err := cmd.Run()
		if err != nil {
			switch e := err.(type) {
			case *exec.ExitError:
				exitCode = e.ExitCode()
			default:
				log.Printf("rspamc error: %v", err)
				return
			}
		} else {
			exitCode = cmd.ProcessState.ExitCode()
		}
		if exitCode != 0 {
			log.Printf("rspamc exited: %d", exitCode)
		}
		stderr := eBuf.String()
		if stderr != "" {
			log.Printf("rspamc stderr: %s", stderr)
		}
		stdout := oBuf.String()
		if stdout != "" {
			log.Printf("rspamc stdout: %s", stdout)
		}

	}
	// debugging delay
	//time.Sleep(1 * time.Second)

}
