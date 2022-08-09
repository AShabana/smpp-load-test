package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	//"constraints"

	"golang.org/x/exp/constraints"
	"golang.org/x/time/rate"

	"github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutext"
)

func NewSmppConnection(port int, ip string, username string, password string, send_rate int) (*SmppConnection, error) {
	lm := rate.NewLimiter(rate.Limit(send_rate), 1) // Max rate of 10/s.
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

func (smppc SmppConnection) sendProc(amount int, respTimeC chan<- []time.Duration, throughputC chan<- []uint32, wg *sync.WaitGroup, from string, to string) {
	r := make([]time.Duration, 0)
	ts := make([]uint32, 0)
	var count uint32
	count = 0
	ticker := time.NewTicker(time.Second)
	done := make(chan bool)
	go func() {
		for {
			log.Println("Inside the sendproc throughput loop")
			select {
			case <-ticker.C:
				log.Printf("The Count per seesion now is %d \n", count)
				ts = append(ts, count)
				atomic.StoreUint32(&count, 0)
			case <-done:
				log.Println("Sending througput data to aggregate Proc")
				log.Println(ts)
				throughputC <- ts
				log.Println("returning from througput data sendProc")
				wg.Done()
				return
			}
		}
	}()
	for i := 0; i < amount; i++ {
		if smppc.healthy {
			
			t := time.Now()
			sm, err := smppc.session.Submit(&smpp.ShortMessage{
				Src:      from,
				Dst:      to,
				Text:     pdutext.Raw("Hello, world!"),
				Register: pdufield.FinalDeliveryReceipt,
			})
			dt := time.Now().Sub(t)
			count++
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
	log.Println("Sending responsetime data to aggregate Proc")
	done <- true
	log.Println(r)
	respTimeC <- r

	//log.Println("Before wg.Done()")
	//time.Sleep(time.Second)
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
		time.Sleep(time.Second)
	}
}

func startBind(N int, ip string, port int, username string, password string, perSecond int) {
	for i := 0; i <= N-1; i++ {
		log.Println("IDX: ", i)
		s, err := NewSmppConnection(port, ip, username, password, perSecond)
		if err != nil {
			log.Fatalf("Failed to create TRX session", err)
		}
		sessions = append(sessions, s)
	}
	log.Println("Sessions: ", sessions)
	log.Println(sessions[0].healthy)
	if !sessions[0].healthy {
		log.Fatal("Not able to bind")
	}
}

func startSubmit(wg *sync.WaitGroup, sess int, count int, from string, to string, collectChan chan []time.Duration, throughputC chan []uint32) {
	for i := 0; i <= sess-1; i++ {
		log.Printf("Spawn new smpp session [%d]\n", i)
		wg.Add(1)
		// Sessoin health check
		go sessions[i].healthProc()
		if sessions[i].healthy {
			go func(i int) {
				startTime := time.Now()
				sessions[i].sendProc(count, collectChan, throughputC, wg, from, to)
				log.Println("*** Overall SyncSend time is: ", time.Now().Sub(startTime))
			}(i)
		} else {
			log.Println("Seem session not healthy !")
		}
	}

}

func calcResults[T constraints.Ordered](data []T) (T, T, T) {
	var sum, max, mini T
	max = data[0]
	mini = data[0]
	for _, i := range data {
		sum += i
		if i > max {
			max = i
		} else if i < mini {
			mini = i
		}
	}
	return sum, max, mini
}

func aggregate[T any](input <-chan []T, output chan<- []T, N int) {
	log.Println("*.*.*.*.*.*. Start stats aggregation proc. *.*.*.*.*.*. ")
	AllResp := make([]T, N-1)
	log.Println("Got new stas from a smpp worker")
	for respArray := range input {
		AllResp = append(AllResp, respArray...)
		log.Println("xxxxxxx Aggregate append done")
	}
	log.Println("Aggregate proc AllResp", AllResp)
	output <- AllResp
	log.Println("*.*.*.*.*.*. Done ")
}
