package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetPrefix("[verifier] ")

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "tally":
		err = runTally(os.Args[2:])
	case "ballot":
		err = runBallot(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("%v", err)
	}
}

func usage() {
	fmt.Println(`verifier — independently verify election results

Usage:
  verifier tally  --election=<id> [fabric flags]
  verifier ballot --election=<id> --ballot-id=<id> [fabric flags]
  verifier serve  --port=8090 [fabric flags]
  verifier help

Fabric flags (required for tally and ballot, and for serve):
  --msp-id=<id>          e.g. ElectoralCommissionMSP
  --endpoint=<host:port> e.g. peer0.electoralcommission.example.com:7051
  --hostname=<name>      TLS server name
  --tls-ca=<path>        TLS CA certificate
  --cert=<path>          user certificate
  --key=<path>           user private key
  --channel=<name>       channel name (default: electionchannel)
  --chaincode=<name>     chaincode name (default: ballot)`)
}
