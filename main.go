// Copyright 2015 go-smpp authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
        "flag"
        "fmt"
        "log"
        "sync"
        "time"

        "golang.org/x/time/rate"

        "github.com/fiorix/go-smpp/smpp"
        "github.com/fiorix/go-smpp/smpp/pdu/pdufield"
        "github.com/fiorix/go-smpp/smpp/pdu/pdutext"
)

var (
        sessions []*SmppConnection

//      responses = make([]time.Duration, 0)
)

// NewSmppConnection : This should build the connection
func NewSmppConnection(port int, ip string, username string, password string) (*SmppConnection, error) {
        lm := rate.NewLimiter(rate.Limit(10), 1) // Max rate of 10/s.
        session := &smpp.Transceiver{
                Addr:        fmt.Sprintf("%s:%d", ip, port),
                User:        username,
                Passwd:      password,
                Handler:     nil, // Handle incoming SM or delivery receipts.
                RateLimiter: lm,  // Optional rate limiter.
        }
        conn := session.Bind()
        if conn == nil {
                log.Fatal("could not connect to the socket.")
        }
        return &SmppConnection{
                socket:  conn,
                session: session,
                healthy: true,
        }, nil
}

// SmppConnection : is struct that represent the smpp
type SmppConnection struct {
        socket  <-chan smpp.ConnStatus
        session *smpp.Transceiver
        healthy bool
}

func (smppc SmppConnection) sendProc(throughput int, collectChan chan<- []time.Duration, wg *sync.WaitGroup) {
        r := make([]time.Duration, 0)
        for i := 0; i < throughput; i++ {
                if smppc.healthy {
                        t := time.Now()
                        sm, err := smppc.session.Submit(&smpp.ShortMessage{
                                Src:      "Sender",
                                Dst:      "201003325373",
                                Text:     pdutext.Raw("Hello, world!"),
                                Register: pdufield.FinalDeliveryReceipt,
                        })
                        dt := time.Now().Sub(t)
                        r = append(r, dt)
                        fmt.Println("Submit_resp duration", dt)
                        if err != nil {
                                fmt.Println(err)
                                return
                        }
                        fmt.Println(sm.RespID())
                } else {
                        log.Println("Will not send session is not healthy")
                        smppc.socket = smppc.session.Bind()
                        log.Println("Trying to re-bind()")
                }
        }
        log.Println("Sending to aggregate Proc")
        log.Println(r)
        collectChan <- r
        //log.Println("Before wg.Done()")
        wg.Done()
        //log.Println("After wg.Done()")
}
func (smppc SmppConnection) healthProc() {
        log.Println("Starting health check proc.")
        for {
                if status := <-smppc.socket; status.Error() != nil {
                        log.Println("Unable to connect, aborting:", status.Error())
                        smppc.healthy = false
                } else {
                        log.Println("Session healthy and its status: ", status.Status())
                        smppc.healthy = true
                }
        }
}

func aggregate(input chan []time.Duration, output chan []time.Duration) {
        //func aggregate(input <-chan []time.Duration, output chan<- []time.Duration) {
        log.Println("Start stats aggregation proc.")
        AllResp := make([]time.Duration, 0)
        log.Println("Got new stas from a smpp worker")
        for respArray := range input {
                AllResp = append(AllResp, respArray...)
        }
        log.Println("Aggregate proc AllResp", AllResp)
        output <- AllResp

}

func main() {
        NumberOfSendSession := flag.Int("sessions", 20, "Number of trx sessions")
        ThroughputPerSession := flag.Int("throughput", 100, "Throughput per session")
        port := flag.Int("port", 2777, "Smpp server port of the smpp server")
        IP := flag.String("ip", "localhost", "ip address of the smpp server")
        username := flag.String("username", "user1", "Bind username")
        password := flag.String("password", "password1", "Bind password")
        flag.Parse()
        var wg sync.WaitGroup
        var collectChan = make(chan []time.Duration)
        var result = make(chan []time.Duration, *NumberOfSendSession)
        wg.Add(*NumberOfSendSession)
        go aggregate(collectChan, result)
        for i := 0; i <= *NumberOfSendSession-1; i++ {
                log.Println("IDX: ", i)
                s, err := NewSmppConnection(*port, *IP, *username, *password)
                if err != nil {
                        log.Fatalf("Failed to create TRX session", err)
                }
                sessions = append(sessions, s)
        }
        fmt.Println("Sessions: ", sessions)
        log.Println(sessions[0].healthy)
        time.Sleep(4 * time.Second)
        overallStartTime := time.Now()
        for i := 0; i < *NumberOfSendSession; i++ {
                log.Printf("Spawn new smpp session [%d]\n", i)
                go sessions[i].healthProc()
                if sessions[i].healthy {
                        go func(i int) {
                                startTime := time.Now()
                                sessions[i].sendProc(*ThroughputPerSession, collectChan, &wg)
                                log.Println("*** Overall SyncSend time is: ", time.Now().Sub(startTime))
                        }(i)
                } else {
                        log.Println("Seem session not healthy !")
                }
        }
        //log.Println("Before wg.Wait()")
        wg.Wait()
        close(collectChan)
        //log.Println("After wg.Wait() and before <-result")
        data := <-result
        var sum, max, mini time.Duration
        mini = 1000 * time.Millisecond
        log.Println(result)
        for _, i := range data {
                sum += i
                if i > max {
                        max = i
                } else if i < mini {
                        mini = i
                }

        }
        //fmt.Println(data)
        log.Printf("Overall Endtime (%v) for (%d) submit_sms", time.Now().Sub(overallStartTime), *NumberOfSendSession**ThroughputPerSession)
        fmt.Println("The average response time is: ", time.Duration(int(sum)/len(data)))
        fmt.Println("The minimum response time is: ", mini)
        fmt.Println("The maximum response time is: ", max)
}
