// Copyright 2015 John Kelley. All rights reserved.
// This source code is licensed under the Simplified BSD License

// Woozle is a simple DNS recursor with the ability to filter out
// certain queries. The author finds this useful for forcing Youtube
// over IPv4 instead of his Hurricane Electric IPv6 tunnel

package main

import (
	"fmt"
	"github.com/miekg/dns"
	"os"
	"os/signal"
	"syscall"
)

const upstreamDNS = "10.10.10.1:53"
var filterDomainAAAA = []string{ "youtube.com.", "googlevideo.com." }

func serve(net string) {
	server := &dns.Server{Addr: ":53", Net: net, TsigSecret: nil}
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Failed to setup the "+net+" server: %s\n", err.Error())
		os.Exit(1)
	}
}

func handleRecurse(w dns.ResponseWriter, m *dns.Msg) {
	fmt.Printf("Recursing for %s %s\n", m.Question[0].Name, dns.TypeToString[m.Question[0].Qtype])
	c := new(dns.Client)
	r, _, e := c.Exchange(m, upstreamDNS)
	if e != nil {
		fmt.Printf("Client query failed: %s\n", e.Error())
	} else {
		w.WriteMsg(r)
	}
}

func filterAAAA(w dns.ResponseWriter, r *dns.Msg) {
	if r.Question[0].Qtype == dns.TypeAAAA {
		// send a blank reply
		m := new(dns.Msg)
		m.SetReply(r)
		w.WriteMsg(m)
		fmt.Printf("Filtering AAAA query for %s\n", m.Question[0].Name)
	} else {
		handleRecurse(w, r)
	}
}

func main() {
	// handler for filtering ipv6 AAAA records
	for _, domain := range filterDomainAAAA {
		dns.HandleFunc(domain, filterAAAA)
	}

	// default handler
	dns.HandleFunc(".", handleRecurse)
//	go serve("tcp")
	go serve("udp")

	// handle signals
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
forever:
	for {
		select {
		case s := <-sig:
			fmt.Printf("\nSignal (%d) received, stopping\n", s)
			break forever
		}
	}
}
