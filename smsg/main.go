// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Rasmus Andersson
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	VERSION   string = "0.1.0"
	BUILDTAG  string = "src" // set at compile time
	DEBUG     bool   = false
	MSGDIR    string // root file directory for messages (env: SMSG_MSGDIR)
	INBOXDIR  string
	OUTBOXDIR string
	DBFILE    string
)

var (
	logger   *log.Logger
	dlog     = func(_ string, _ ...interface{}) {}
	db       DB
	msgsync  MessageSyncer
	progname string
)

func dlog1(format string, arg ...interface{}) {
	logger.Printf("[debug] "+format, arg...)
}

func errlog(format string, arg ...interface{}) {
	logger.Printf("[error] "+format, arg...)
}

func warnlog(format string, arg ...interface{}) {
	logger.Printf("[warning] "+format, arg...)
}

func cmd_version() {
	fmt.Printf("smsg %s (build %s)\n", VERSION, BUILDTAG)
	os.Exit(0)
}

func main() {
	const usagefmt = `
Usage: %s [options] <command>
Commands:
  list         List messages in your inbox (default)
  read <id>    Read a message
  send <file>  Send a message
  serve <dir>  Start a smolmsg server, storing state in <dir>
Options:
`
	progname = os.Args[0]
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, strings.TrimSpace(usagefmt)+"\n", progname)
		flag.PrintDefaults()
	}
	flag.StringVar(&MSGDIR, "C", "",
		"Set messages root directory.\n"+
			"Overrides environment variable SMSG_MSGDIR.\n"+
			"Defaults to ~/.smolmsg")
	opt_version := flag.Bool("version", false, "Print version and exit")
	flag.BoolVar(&DEBUG, "D", false, "Enable debug mode")
	flag.Parse()

	logger = log.New(os.Stdout, "▎", 0)
	if DEBUG {
		dlog = dlog1
	}

	if *opt_version {
		cmd_version()
	}

	// set MSGDIR
	if MSGDIR == "" {
		MSGDIR = os.Getenv("SMSG_MSGDIR")
		if MSGDIR == "" {
			home, err := os.UserHomeDir()
			must(err)
			MSGDIR = filepath.Join(home, ".smolmsg")
		}
	}
	var err error
	MSGDIR, err = filepath.Abs(MSGDIR)
	must(err)
	dlog("MSGDIR=%q", MSGDIR)
	os.Setenv("SMSG_MSGDIR", MSGDIR)
	INBOXDIR = filepath.Join(MSGDIR, "inbox")
	OUTBOXDIR = filepath.Join(MSGDIR, "outbox")
	DBFILE = filepath.Join(MSGDIR, "smsg.db")
	must(os.MkdirAll(INBOXDIR, 0700))
	must(os.MkdirAll(OUTBOXDIR, 0700))
	must(os.Chdir(MSGDIR))

	// open database
	must(db.Open())
	RegisterExitHandler(db.Close)

	// start sync process
	msgsync.Start()

	// call command function
	var cmd = "list"
	var cmdargs []string
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
		cmdargs = flag.Args()[1:]
	}
	switch cmd {
	case "list", "ls", "l":
		cmd_list(cmdargs...)
	case "read", "r":
		cmd_read(cmdargs...)
	case "send":
		cmd_send(cmdargs...)
	case "serve":
		cmd_serve(cmdargs...)
	case "version":
		cmd_version()
	case "help":
		flag.Usage()
		os.Exit(0)
	default:
		fatalf("Unknown command %q\nSee %s -h for help", cmd, os.Args[0])
	}

	// TODO: only if no serve is going on
	Shutdown(0)

	// channel closes when all exit handlers have completed
	<-ExitCh
}

func must(err error) {
	if err != nil {
		fatalf(err)
	}
}
