package main

import (
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"fmt"
	"os"
	"os/signal"
	"net"
	"log"
	"time"
	"errors"
	"syscall"
)
const (
	Delay = 1000 // ms
	Client = "" // sets to default source IP
	Ttl = 55
	Timeout = 5000 // ms
)

var (
	seq = 1
	deadline = time.Now()
	before = time.Now()
	inflight = false

	totalrtt = 0
	dropped = 0
	received = 0

	max = 0
	min = int(^uint(0) >> 1) // MaxInt
)

func ping() {
	addr := os.Args[1]
	
	ipaddr, err := net.ResolveIPAddr("ip4", addr)
	if err != nil {
		log.Fatal(err)
	}
	
	/* Listen for incoming ICMP packets on local machine */
	conn, err := icmp.ListenPacket("ip4:icmp", Client)
	
	if err != nil {
		log.Println("Please try running go build && sudo ./ping [hostname]")
		log.Fatal(err)
	}
	defer conn.Close()
	
	/* Set TTL for packet */
	conn.IPv4PacketConn().SetTTL(Ttl)
	
	/* Continue sending and receiving ICMP packets */
	for {
		/* Construct ICMP Message */
		msg := icmp.Message {
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo {
				ID: os.Getpid() & 0xffff, Seq: seq,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}
	
		msgbuf, err := msg.Marshal(nil)
		if err != nil {
			log.Fatal(err)
		}

		/* Send new packet if one not already in flight */
		if !inflight {
			before = time.Now()

			_, err = conn.WriteTo(msgbuf, &net.IPAddr{IP: net.ParseIP(ipaddr.String()), Zone: "eth0"}); 
			if err != nil {
				log.Fatal(err)
			}

			seq++
			inflight = true
			deadline = time.Now().Add(time.Duration(Timeout) * time.Millisecond)
		}

		/* Receive and Parse message */
		res := make([]byte, 1500)
		conn.SetReadDeadline(deadline)
		length, sourceIP, err := conn.ReadFrom(res)
		if err != nil {
			log.Fatal(err)
		}
	
		resmsg, err := icmp.ParseMessage(1, res[:length])
		if err != nil {
			if errors.Is(err, syscall.ETIMEDOUT) {
				fmt.Println("Timeout\n")
				inflight = false
				dropped++
				continue
			}
			log.Fatal(err)
		}
		
		/* Check is message is of type ICMPTypeEchoReply */
		switch resmsg.Type {
		case ipv4.ICMPTypeEchoReply:
			received++
			inflight = false

			rtt := time.Now().Sub(before)
			us := int(rtt.Microseconds())
			if max < us {
				max = us
			}
			if min > us {
				min = us
			}
			totalrtt += us

			res_seq := resmsg.Body.(*icmp.Echo).Seq
			fmt.Printf("%d bytes from %v: icmp_seq=%d ttl=%d time=%v\n", length, sourceIP, res_seq, Ttl, rtt)
			
			time.Sleep(time.Duration(Delay) * time.Millisecond)
		default:
			// not EchoReply
			// ignore
		}
	
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go build && sudo ./ping [hostname]")
		return
	}

	// Track time at program start (statistics)
	start := time.Now()

	/* Create channel to handle signal for program termination */
	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan,
		syscall.SIGINT)

	/* Ping */
	go ping()
		

	_ = <-signal_chan // Received SIGINT

	/* Construct statistics */
	avg := float64(totalrtt) / float64(received) / 1000
	fmin := float64(min) / 1000
	fmax := float64(max) / 1000
	transmitted := seq-1
	pkt_loss := dropped / transmitted

	fmt.Printf("\n--- ping statistics ---\n%d packets transmitted, %d received, %d%% packet loss, time %dms\nrtt min/avg/max: %.3f/%.3f/%.3f ms\n", 
		transmitted, received, pkt_loss, time.Now().Sub(start).Milliseconds(), fmin, avg, fmax);
}

