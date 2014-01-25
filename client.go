package main

import (
	"bufio"
	"fmt"
	"log"
	"mylib"
	"net/rpc"
	"os"
	"strconv"
)

func main() {
	fmt.Println("*** Welcome to the Internet ***")
	fmt.Print("Connecting to your bank's server : 127.0.0.1:90...")

	in := bufio.NewReader(os.Stdin)
	portEnd, _ := in.ReadString('\n')

	bank, err := rpc.DialHTTP("tcp", "localhost:90"+portEnd[:len(portEnd)-1])
	if err != nil {
		log.Fatal("dialing:", err)
	}

	fmt.Println("...connected !")
	fmt.Print("Enter your client ID :\n> ")
	scid, _ := in.ReadString('\n')
	cid, _ := strconv.Atoi(scid[:len(scid)-1])

	moneyReq := &mylib.MoneyRequest{cid}
	var moneyResp mylib.MoneyResponse
	call := bank.Go("Ressource.Money", moneyReq, &moneyResp, nil)
	_ = <-call.Done // wait for it
	fmt.Printf("Money: %+v\n", moneyResp)

	fmt.Print("Enter your friend's IBAN :\n> ")
	siban, _ := in.ReadString('\n')
	iban, _ := strconv.Atoi(siban[:len(siban)-1])

	fmt.Print("Enter the amount of money to transfer :\n> ")
	samount, _ := in.ReadString('\n')
	amount, _ := strconv.Atoi(samount[:len(samount)-1])

	transferReq := &mylib.TransferRequest{cid, iban, amount}
	var transferResp mylib.TransferResponse
	call = bank.Go("Ressource.Transfer", transferReq, &transferResp, nil)
	_ = <-call.Done // wait for it
	fmt.Printf("Transfer: %+v\n", transferResp)
}
