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
	"syscall"
	"time"
)

type Ressource int

var logg chan string

func (r *Ressource) Coord(req *mylib.CoordRequest, resp *mylib.CoordResponse) error {
	tid := time.Now().UnixNano()
	logg <- fmt.Sprintf("[%d] request: %+v\n", tid, req)

	srcBank, err := rpc.DialHTTP("tcp", "localhost:90"+strconv.Itoa(req.Src/100))
	if err != nil {
		fmt.Println("Bank", req.Src/100, "is unreachable.")
		resp.Ok = false
		return nil
	}
	destBank, err := rpc.DialHTTP("tcp", "localhost:90"+strconv.Itoa(req.Dest/100))
	if err != nil {
		fmt.Println("Bank", req.Dest/100, "is unreachable.")
		resp.Ok = false
		return nil
	}

	sudoTakeReq := &mylib.SudoRequest{req.Src % 100, -req.Amount, tid}
	sudoGiveReq := &mylib.SudoRequest{req.Dest % 100, req.Amount, tid}
	var sudoTakeResp mylib.SudoResponse
	var sudoGiveResp mylib.SudoResponse
	takeCall := srcBank.Go("Ressource.Sudo", sudoTakeReq, &sudoTakeResp, nil)
	giveCall := destBank.Go("Ressource.Sudo", sudoGiveReq, &sudoGiveResp, nil)
	_ = <-takeCall.Done // wait for it
	_ = <-giveCall.Done // wait for it
	vote := sudoGiveResp.Vote && sudoTakeResp.Vote
	logg <- fmt.Sprintf("[%d] decision: %t\n", tid, vote)

	orderReq := &mylib.OrderRequest{vote, tid}
	var srcOrderResp mylib.OrderResponse
	var destOrderResp mylib.OrderResponse
	srcCall := srcBank.Go("Ressource.Order", orderReq, &srcOrderResp, nil)
	destCall := destBank.Go("Ressource.Order", orderReq, &destOrderResp, nil)
	go func() {
		_ = <-srcCall.Done  // wait for it
		_ = <-destCall.Done // wait for it
	}()

	resp.Ok = vote
	return nil
}

func WriteLogs(messages <-chan string) <-chan bool {
	done := make(chan bool, 1)
	go func() {
		outFile, err := os.Create("coord.log")
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
			fmt.Println("Logging:", s)
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

func main() {
	logg = make(chan string)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	fmt.Println("Booting BANK Coordinator (TM) : Done.")

	ressource := new(Ressource)
	rpc.Register(ressource)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":9000")
	if e != nil {
		log.Fatal("listen error:", e)
	}

	go http.Serve(l, nil)
	loggerDone := WriteLogs(logg)

	_ = <-sigs // wait for it (Ctrl-C)
	close(logg)
	_ = <-loggerDone // wait logger confirmation
	fmt.Println("Off.")
}
