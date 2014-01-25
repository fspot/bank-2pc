package main

import (
	"bufio"
	"fmt"
	"log"
	"mylib"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type Account struct {
	Money int
	lock  sync.RWMutex
}

type Ressource struct {
	BankID   int
	Accounts map[int]*Account
}

type TransactionData struct {
	sudoc  chan mylib.SudoRequest
	sudorc chan bool
	ordc   chan mylib.OrderRequest
	ordrc  chan bool
}

type TransactionTable struct {
	table map[int64]TransactionData
	lock  sync.RWMutex
}

func (r *Ressource) Money(req *mylib.MoneyRequest, resp *mylib.MoneyResponse) error {
	fmt.Printf("Rcv MoneyRequest: %+v\n", req)
	// logg <- "MoneyRequest from: " + strconv.Itoa(req.Cid) + "\n"
	account := r.Accounts[req.Cid]
	account.lock.RLock()
	resp.Money = account.Money
	account.lock.RUnlock()
	resp.Ok = true
	return nil
}

func (r *Ressource) Transfer(req *mylib.TransferRequest, resp *mylib.TransferResponse) error {
	fmt.Printf("Rcv TransferRequest: %+v\n", req)
	// logg <- "TransferRequest from: " + strconv.Itoa(req.Cid) + "\n"

	resp.Ok = false
	if req.FriendIBAN/100 == r.BankID {
		// the transfer is local
		accountSrc, srcFound := r.Accounts[req.Cid]
		accountDst, dstFound := r.Accounts[req.FriendIBAN%100]
		if srcFound && dstFound {
			// OK
			accountSrc.lock.Lock()
			if accountSrc.Money-req.Amount >= 0 {
				logg <- fmt.Sprintf("[LOCAL] done: %d to %d (%d €)\n",
					req.Cid,
					req.FriendIBAN%100,
					req.Amount)
				accountSrc.Money -= req.Amount
				accountDst.lock.Lock()
				accountDst.Money += req.Amount
				accountDst.lock.Unlock()
				resp.Ok = true
			}
			accountSrc.lock.Unlock()
		}
	} else {
		// coordinator to the rescue !
		coord, err := rpc.DialHTTP("tcp", "localhost:9000")
		if err != nil {
			log.Fatal("dialing:", err)
		}

		coordReq := &mylib.CoordRequest{100*r.BankID + req.Cid, req.FriendIBAN, req.Amount}
		var coordResp mylib.CoordResponse
		call := coord.Go("Ressource.Coord", coordReq, &coordResp, nil)
		_ = <-call.Done // wait for it
		fmt.Printf("Coord: %+v\n", coordResp)

		resp.Ok = coordResp.Ok
	}
	return nil
}

func (r *Ressource) Sudo(req *mylib.SudoRequest, resp *mylib.SudoResponse) error {
	fmt.Printf("Rcv SudoRequest: %+v\n", req)
	sudoChan := make(chan mylib.SudoRequest)
	sudoChanResp := make(chan bool)
	orderChan := make(chan mylib.OrderRequest)
	orderChanResp := make(chan bool)
	td := TransactionData{sudoChan, sudoChanResp, orderChan, orderChanResp}
	transactionTable.lock.Lock()
	transactionTable.table[req.TransactionID] = td
	transactionTable.lock.Unlock()
	go transactionManager(sudoChan, sudoChanResp, orderChan, orderChanResp, r)

	fmt.Println("voting...")

	td.sudoc <- *req
	fmt.Println("... ...")
	vote := <-td.sudorc
	resp.Vote = vote
	fmt.Println("...voted.")
	return nil
}

func (r *Ressource) Order(req *mylib.OrderRequest, resp *mylib.OrderResponse) error {
	fmt.Printf("Rcv OrderRequest: %+v\n", req)
	time.Sleep(20 * time.Second)
	fmt.Println("20s passed ? Processing order...")

	transactionTable.lock.RLock()
	td := transactionTable.table[req.TransactionID]
	transactionTable.lock.RUnlock()

	td.ordc <- *req
	fmt.Println("... ... ...")
	ok := <-td.ordrc
	resp.Ok = ok
	fmt.Println("...order processed.")
	return nil
}

func transactionManager(
	sudoCh <-chan mylib.SudoRequest,
	sudoChResp chan<- bool,
	orderCh <-chan mylib.OrderRequest,
	orderChResp chan<- bool,
	r *Ressource) {
	// Ew.
	sudoReq := <-sudoCh
	logg <- fmt.Sprintf("[%d] requested: %d€ to %d\n",
		sudoReq.TransactionID,
		sudoReq.Amount,
		sudoReq.Cid)
	account, found := r.Accounts[sudoReq.Cid]
	if found {
		account.lock.Lock()
		sudoChResp <- (account.Money+sudoReq.Amount >= 0)
	} else {
		sudoChResp <- false
	}
	orderReq := <-orderCh
	fmt.Printf("orderCh: %+v\n", orderReq)
	if orderReq.Commit {
		logg <- fmt.Sprintf("[%d] COMMIT;\n", orderReq.TransactionID)
		account.Money += sudoReq.Amount
	} else {
		logg <- fmt.Sprintf("[%d] ROLLBACK;\n", orderReq.TransactionID)
	}
	if found {
		account.lock.Unlock()
	}
	orderChResp <- true
}

func writeLogs(bank_id string, messages <-chan string) <-chan bool {
	done := make(chan bool, 1)
	go func() {
		outFile, err := os.Create("bank_" + bank_id + ".log")
		if err != nil {
			panic(err)
		}
		defer func() {
			fmt.Println("Deferred close...")
			outFile.Close()
		}()

		writer := bufio.NewWriter(outFile)
		for {
			s, ok := <-messages
			if !ok {
				break
			}
			fmt.Printf("Logging: %+v\n", s)
			_, err := writer.WriteString(s)
			writer.Flush()
			if err != nil {
				panic(err)
			}
		}

		done <- true
	}()
	return done
}

var logg chan string
var transactionTable TransactionTable

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	logg = make(chan string)

	transactionTable = TransactionTable{table: map[int64]TransactionData{}}

	fmt.Print("Booting BANK server N° :\n> ")
	in := bufio.NewReader(os.Stdin)
	portEnd, _ := in.ReadString('\n')
	bank_id := portEnd[:len(portEnd)-1]
	fmt.Println("...Done.")

	ressource := new(Ressource)
	ressource.BankID, _ = strconv.Atoi(bank_id)
	ressource.Accounts = map[int]*Account{
		1: &Account{Money: 101},
		2: &Account{Money: 202},
		3: &Account{Money: 303},
		4: &Account{Money: 404},
		5: &Account{Money: 505},
	}
	rpc.Register(ressource)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":90"+bank_id)
	if e != nil {
		log.Fatal("listen error:", e)
	}

	go http.Serve(l, nil)
	loggerDone := writeLogs(bank_id, logg)

	_ = <-sigs // wait for it (Ctrl-C)
	// here I should kill all transaction goroutines
	close(logg)
	_ = <-loggerDone // wait logger confirmation
	fmt.Println("Ciao !")
}
