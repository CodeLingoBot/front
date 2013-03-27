// Copyright 2013 Francisco Souza. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"github.com/fsouza/lb"
	"github.com/howeyc/fsnotify"
	"log"
	"os"
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
	s := Server{}
	var err error
	s.rules, err = s.loadRules(ruleFile)
	if err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Warning: fronted is not watching rule file %q. Reason: %s", ruleFile, err)
		return &s, nil
	}
	err = w.Watch(ruleFile)
	if err != nil {
		log.Printf("Warning: fronted is not watching rule file %q. Reason: %s", ruleFile, err)
		return &s, nil
	}
	go func() {
		for {
			select {
			case e := <-w.Event:
				if e.IsModify() {
					if rules, err := s.loadRules(ruleFile); err == nil {
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

func (s *Server) loadRules(file string) ([]Rule, error) {
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

func main() {}
