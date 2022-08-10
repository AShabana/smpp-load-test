package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	sessions []*SmppConnection

//      responses = make([]time.Duration, 0)
)

func main() {

	numberOfSendSession := flag.Int("sessions", 20, "Number of trx sessions")
	amount := flag.Int("amount", 20, "Total sms per session")
	throughputPerSession := flag.Int("throughput", 100, "Throughput per session")
	port := flag.Int("port", 2777, "Smpp server port of the smpp server")
	IP := flag.String("ip", "localhost", "ip address of the smpp server")
	from := flag.String("sender", "Cequens", "Caller id ")
	to := flag.String("recepient", "966534510820", "Mobile number")
	username := flag.String("username", "user1", "Bind username")
	password := flag.String("password", "password1", "Bind password")
	systemType := flag.String("systemtype", "77002", "System Type")

	flag.Parse()

	var wg sync.WaitGroup
	var collectChan = make(chan []time.Duration)
	var throughputC = make(chan []uint32, 1)
	var responeTimeResult = make(chan []time.Duration, 1)
	var throughputResult = make(chan []uint32, 1)
	go aggregate(collectChan, responeTimeResult, *numberOfSendSession)
	go aggregate(throughputC, throughputResult, *numberOfSendSession)
	// Smpp Bind startBind(N int, ip string, port string, username string, password string, perSecond int)
	startBind(*numberOfSendSession, *IP, *port, *username, *password, *systemType, *throughputPerSession)
	log.Println("Test will start within 4 seconds...")
	time.Sleep(4 * time.Second)
	overallStartTime := time.Now()
	// Smpp Submit
	startSubmit(&wg, *numberOfSendSession, *amount, *from, *to, collectChan, throughputC)
	//log.Println("Before wg.Wait()")
	wg.Wait()
	close(collectChan)
	close(throughputC)
	log.Println("After wg.Wait() and before <-responeTimeResult")
	wg.Add(2)
	var sum, max, mini time.Duration
	var sumT, maxT, miniT uint32
	var dataResponseTime []time.Duration
	var dataThroughput []uint32
	var average int
	go func() {
		log.Println("Waiting responseTime Results")
		dataResponseTime = <-responeTimeResult
		sum, max, mini = calcResults(dataResponseTime)
		wg.Done()
		log.Println("responseTime Results Done")
	}()
	go func() {
		log.Println("Waiting Throughput Results")
		dataThroughput = <-throughputResult
		sumT, maxT, miniT = calcResults(dataThroughput)
		wg.Done()
		log.Println("responseTime Throughput Done")
	}()
	wg.Wait()
	log.Println("After Second wg.Done")
	//fmt.Println(data)
	log.Printf("Overall Endtime (%v) for (%d) submit_sms", time.Now().Sub(overallStartTime), *numberOfSendSession**throughputPerSession)
	fmt.Println("The average response time is: ", time.Duration(int(sum)/len(dataResponseTime)))
	fmt.Println("The minimum response time is: ", time.Duration(mini))
	fmt.Println("The maximum response time is: ", time.Duration(max))

	if sumT == 0 {
		average = 1
		miniT = 1
		maxT = 1
	} else {
		average = int(sumT) / len(dataThroughput)
	}

	fmt.Println("The average per second: ", average)
	fmt.Println("The minimum per second: ", miniT)
	fmt.Println("The maximum per second: ", maxT)
}
