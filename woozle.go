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
	"sort"
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
type PStats *DomainStats
type ByFreq []*DomainStats
var sortedStats = make(ByFreq, 100)[0:0]
func (s ByFreq) Len() int {
	return len(s)
}
func (s ByFreq) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ByFreq) Less(i, j int) bool {
//	fmt.Printf("len: %d. %i > %j\n", len(s), i, j)
	return s[i].frequency > s[j].frequency
}
func (s ByFreq) Append(stats *DomainStats) ByFreq {
	l := len(s)
//	fmt.Printf("len: %d, cap %d\n", l, cap(s))
	if l + 1 > cap(s) {
		newSlice := make(ByFreq, l*2)
		copy(newSlice, s)
		s = newSlice
	}
	s = s[0:l+1]
	s[l] = stats

	return s
}
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

	// print up to top 10 domains
	l := len(sortedStats)
	if l > 10 {
		l = 10
	}
	for i := 0; i < l; i++ {
		fmt.Printf("%25s: %3d queries", sortedStats[i].domain, sortedStats[i].frequency)
		if sortedStats[i].filtered > 0 {
			fmt.Printf(", %3d dropped", sortedStats[i].filtered)
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
		fmt.Printf("%s retrying query for '%s'\n", time.Now().Format("01/02 15:04"), m.Question[0].Name);
		time.Sleep(time.Millisecond * 10);
		r, _, e = c.Exchange(m, upstreamDNS)
		if e != nil {
			fmt.Printf("%s Client query failed for '%s': %s\n", time.Now().Format("01/02 15:04"), m.Question[0].Name, e.Error())
			return
		}
	}
	w.WriteMsg(r)
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
			sortedStats = sortedStats.Append(stats[domain])
		}
		stats[domain].frequency++
		if query.filtered {
			stats[domain].filtered++
		}
		stats[domain].queries[query.queryType] += 1
		if stats[domain].frequency > 1 {
			sort.Sort(sortedStats)
		}
		//fmt.Printf("%s type %s (%d, %d)\n", query.domain, query.queryType, stats[query.domain].frequency, stats[query.domain].queries[query.queryType])
	}
	fmt.Printf("stats engine stopping\n");
}

func main() {
	// init update
	timeStarted = time.Now()
	lastSigInt := timeStarted.Add(time.Second * -31)

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
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
forever:
	for s := range sig {
		fmt.Printf("\nUptime %s, %d queries performed", time.Since(timeStarted).String(), totalQueries)
		if time.Since(lastSigInt).Seconds() < 30 {
			fmt.Printf("\nSignal (%d) received, stopping\n", s)
			break forever
		} else {
			fmt.Printf(", send SIGINT again within 30s to quit\n")
			lastSigInt = time.Now();
			dispStats()
		}
	}
}
