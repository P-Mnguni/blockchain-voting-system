package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	cc, err := contractapi.NewChaincode(&BallotContract{})
	if err != nil {
		log.Panicf("Error creating ballot chaincode: %v", err)
	}

	if err := cc.Start(); err != nil {
		log.Panicf("Error starting ballot chaincode: %v", err)
	}
}
