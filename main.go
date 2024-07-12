package main

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strconv"
)

func main() {
	// check dante version
	// check dante service file
	// check dante config file

	users := make(map[string]int64)

	go func() {
		panic(http.ListenAndServe(":9090", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(users)
		})))
	}()

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		bytes, err := reader.ReadBytes('\n')
		if err != nil {
			panic(err)
		}
		percentIndex := slices.Index(bytes, '%')
		if percentIndex < 0 {
			continue
		}
		atIndex := slices.Index(bytes[percentIndex:], '@') + percentIndex
		if atIndex < 0 {
			continue
		}
		username := bytes[percentIndex+1 : atIndex]
		openParenIndex := slices.Index(bytes[atIndex:], '(') + atIndex
		if openParenIndex < 0 {
			continue
		}
		closeParenIndex := slices.Index(bytes[openParenIndex:], ')') + openParenIndex
		if closeParenIndex < 0 {
			continue
		}
		count := bytes[openParenIndex+1 : closeParenIndex]
		countInt, err := strconv.ParseInt(string(count), 10, 64)
		if err != nil {
			panic(err)
		}
		if _, ok := users[string(username)]; ok {
			users[string(username)] += countInt
		} else {
			users[string(username)] = countInt
		}
	}
}
