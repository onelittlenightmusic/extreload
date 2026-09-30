// extreload reloads an unpacked Chrome extension from the command line.
//
// Chrome reads an unpacked extension from its folder only when it is reloaded,
// and the only way it offers is the ↻ on chrome://extensions. extreload does it
// through a small helper extension (extension/): it can switch another
// extension off and on again with chrome.management, which reloads it from
// disk, and then reload the tabs that ran its old content scripts.
//
// The helper keeps a WebSocket open to `extreload serve`, a small server on
// localhost; `extreload reload` asks that server, which asks every connected
// helper (one per Chrome profile it is installed in) and prints what each did.
// The server is started on first use.
package main

import (
	"fmt"
	"os"
)

var version = "dev"

func usage() {
	fmt.Fprint(os.Stderr, `extreload — reload an unpacked Chrome extension from the command line

Usage:
  extreload reload --name <name> | --id <id> [--tabs <url pattern>]... [--profile <label>]
  extreload list      the unpacked extensions each connected Chrome has
  extreload status    whether the server is up, and which Chromes are connected
  extreload serve     run the server in the foreground (normally started for you)
  extreload version

The helper extension (extension/ in this repository) must be loaded in Chrome.
Port: 7988, or $EXTRELOAD_PORT.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "reload":
		err = runReload(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	case "status":
		err = runStatus(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "extreload: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "extreload:", err)
		os.Exit(1)
	}
}
