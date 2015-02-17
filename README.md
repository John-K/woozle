# woozle
A simple recursing DNS server with the ability to filter out IPv6 queries for specified domains

This project was started because I was having trouble streaming Youtube videos over my Hurricane Electric TunnelBroker IPv6 tunnel and I couldn't find a simple recursing DNS server that let me filter out AAAA queries selectively.

# Installation
go get github.com/John-K/woozle

## ToDo
 * Add commandline argument support
   * specifying upstream DNS servers 
   * specifying hosts to filter AAAA queries
 * ~~Add support for keeping statistics and dumping to stderr on SIGINFO~~
 * Add support for filtering other types of DNS queries - which ones?
 * Better error handling
