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
	"strings"
	"time"
)

// ================================================
// Configurables - TODO: move to CLI arg processing
// ================================================
const upstreamDNS = "10.10.10.1:53"
var filterDomainAAAA = []string{ "youtube.com.", "googlevideo.com." }

// ================================================
// Globals for tracking stats and caching
// ================================================
type DNSQuery struct {
	domain string
	queryType string
	filtered bool
}

type DomainStats struct {
	domain string
	frequency int
	filtered int
	queries map[string]int
}

var statPipe chan DNSQuery
var stats = map[string]*DomainStats{}
var totalQueries = 0
var timeStarted time.Time

// ================================================
// Helper functions
// ================================================

func serve(net string) {
	server := &dns.Server{Addr: ":53", Net: net, TsigSecret: nil}
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Failed to setup the "+net+" server: %s\n", err.Error())
		os.Exit(1)
	}
}

func getRootFromDomain(domain string) string {
	var root string
	components := strings.Split(domain, ".")
	idx := len(components)
	if idx > 2 {
		root = components[idx-3] + "." + components[idx-2]
	}
	return root
}

func dispStats() {
	fmt.Printf("\nQuery Statistics:\n")

	for d, s := range stats {
		fmt.Printf("%25s: %3d queries", d, s.frequency)
		if s.filtered > 0 {
			fmt.Printf(", %3d dropped", s.filtered)
		}
		fmt.Println()
	}
}

// ================================================
// Meat and potatoes
// ================================================

func handleRecurse(w dns.ResponseWriter, m *dns.Msg) {
	// collect stats
	statPipe <- DNSQuery{m.Question[0].Name, dns.TypeToString[m.Question[0].Qtype], false}

	// pass the query on to the upstream DNS server
	c := new(dns.Client)
	r, _, e := c.Exchange(m, upstreamDNS)
	if e != nil {
		fmt.Printf("%s Client query failed: %s\n", time.Now().Format("01/02 15:04"), e.Error())
	} else {
		w.WriteMsg(r)
	}
}

func filterAAAA(w dns.ResponseWriter, r *dns.Msg) {
	if r.Question[0].Qtype == dns.TypeAAAA {
		// collect stats
		statPipe <- DNSQuery{r.Question[0].Name, "AAAA", true}

		// send a blank reply
		m := new(dns.Msg)
		m.SetReply(r)
		w.WriteMsg(m)
	} else {
		handleRecurse(w, r)
	}
}

func handleStats(queryChan <-chan DNSQuery) {
	var domain string
	for query := range queryChan {
		totalQueries++
		domain = getRootFromDomain(query.domain)
		if nil == stats[domain] {
			stats[domain] = &DomainStats{domain:domain}
			stats[domain].queries = make(map[string]int)
		}
		stats[domain].frequency++
		if query.filtered {
			stats[domain].filtered++
		}
		stats[domain].queries[query.queryType] += 1
		//fmt.Printf("%s type %s (%d, %d)\n", query.domain, query.queryType, stats[query.domain].frequency, stats[query.domain].queries[query.queryType])
	}
	fmt.Printf("stats engine stopping\n");
}

func main() {
	// init update
	timeStarted = time.Now()

	// setup and kickoff stats collection
	statPipe = make(chan DNSQuery, 10)
	go handleStats(statPipe)

	// handler for filtering ipv6 AAAA records
	for _, domain := range filterDomainAAAA {
		dns.HandleFunc(domain, filterAAAA)
	}

	// default handler
	dns.HandleFunc(".", handleRecurse)

	// start server(s)
	//go serve("tcp")
	go serve("udp")

	// handle signals
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)
forever:
	for s := range sig {
		if s == syscall.SIGUSR1 {
			fmt.Printf("Uptime %s, %d queries performed, send SIGUSR2 for details\n", time.Since(timeStarted).String(), totalQueries)
		} else if s == syscall.SIGUSR2 {
			dispStats()
		} else {
			fmt.Printf("Uptime %s, %d queries performed, send SIGUSR2 for details\n", time.Since(timeStarted).String(), totalQueries)
			fmt.Printf("\nSignal (%d) received, stopping\n", s)
			break forever
		}
	}
}
