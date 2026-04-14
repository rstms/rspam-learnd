package sample

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
)

const Version = "0.0.6"

type Sample struct {
	Username string
	Class    string
	Domains  []string
	Filename string
	verbose  bool
}

func NewSample(class, username string, domains []string, filename string) (*Sample, error) {

	s := Sample{
		Class:    class,
		Username: username,
		Domains:  domains,
		Filename: filename,
	}
	return &s, nil
}

func (s *Sample) Submit() error {
	verbose := ViperGetBool("verbose")
	if verbose {
		log.Printf("Submitting %s %s %v %s\n", s.Username, s.Class, s.Domains, s.Filename)
	}
	for _, domain := range s.Domains {
		args := []string{}
		if verbose {
			args = []string{"-v"}
		}
		args = append(args, []string{"-d", fmt.Sprintf("%s@%s", s.Username, domain), "learn_" + s.Class, s.Filename}...)
		cmd := exec.Command("rspamc", args...)
		var oBuf bytes.Buffer
		var eBuf bytes.Buffer
		cmd.Stdout = &oBuf
		cmd.Stderr = &eBuf
		exitCode := -1
		if verbose {
			log.Printf("%s\n", cmd.String())
		}
		err := cmd.Run()
		if err != nil {
			switch e := err.(type) {
			case *exec.ExitError:
				exitCode = e.ExitCode()
			default:
				return Fatalf("rspamc error: %v", err)
			}
		} else {
			exitCode = cmd.ProcessState.ExitCode()
		}
		stderr := eBuf.String()
		if stderr != "" {
			log.Printf("rspamc stderr: %s", stderr)
		}
		stdout := oBuf.String()
		if stdout != "" {
			log.Printf("rspamc stdout: %s", stdout)
		}
		if exitCode != 0 {
			return Fatalf("rspamc exited: %d", exitCode)
		}

	}

	err := os.Remove(s.Filename)
	if err != nil {
		return Fatal(err)
	}
	// debugging delay
	//time.Sleep(1 * time.Second)
	return nil
}
