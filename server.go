// Copyright 2013 Francisco Souza. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"github.com/fsouza/lb"
	"github.com/howeyc/fsnotify"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type jsonRule struct {
	Domain   string
	Backends []string
}

type Rule struct {
	Domain  string
	Backend *lb.LoadBalancer
}

type Server struct {
	rules []Rule
	rmut  sync.RWMutex
}

func NewServer(ruleFile string) (*Server, error) {
	rules, err := loadRules(ruleFile)
	if err != nil {
		return nil, err
	}
	s := Server{rules: rules}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Warning: frontend server is not watching rule file %q. Reason: %s", ruleFile, err)
		return &s, nil
	}
	err = w.Watch(ruleFile)
	if err != nil {
		log.Printf("Warning: frontend server is not watching rule file %q. Reason: %s", ruleFile, err)
		return &s, nil
	}
	go func() {
		for {
			select {
			case e := <-w.Event:
				if e.IsModify() {
					if rules, err := loadRules(ruleFile); err == nil {
						s.rmut.Lock()
						s.rules = rules
						s.rmut.Unlock()
					}
				}
			case <-w.Error:
			}
		}
	}()
	return &s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Host == "" {
		http.Error(w, "Missing Host header", http.StatusBadRequest)
		return
	}
	var rule Rule
	s.rmut.RLock()
	for _, r := range s.rules {
		if req.Host == r.Domain || strings.HasSuffix(req.Host, "."+r.Domain) {
			rule = r
			break
		}
	}
	s.rmut.RUnlock()
	if rule.Domain == "" {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}
	rule.Backend.ServeHTTP(w, req)
}

func loadRules(file string) ([]Rule, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rs []jsonRule
	err = json.NewDecoder(f).Decode(&rs)
	if err != nil {
		return nil, &invalidRuleError{err}
	}
	rules := make([]Rule, len(rs))
	for i, r := range rs {
		balancer, err := lb.NewLoadBalancer(r.Backends...)
		if err != nil {
			return nil, &invalidRuleError{err}
		}
		rules[i] = Rule{Domain: r.Domain, Backend: balancer}
	}
	return rules, nil
}

type invalidRuleError struct {
	err error
}

func (e *invalidRuleError) Error() string {
	return "Invalid rule file: " + e.err.Error()
}

func main() {
	var listen, rulesFile string
	flag.StringVar(&listen, "listen", ":8080", "Address to listen to HTTP requests.")
	flag.StringVar(&rulesFile, "rules", "", "JSON file to load rules from. You must provide it.")
	flag.Parse()
	if rulesFile == "" {
		flag.Usage()
		os.Exit(2)
	}
	server, err := NewServer(rulesFile)
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", server)
	if err := http.ListenAndServe(listen, nil); err != nil {
		log.Fatal(err)
	}
}
