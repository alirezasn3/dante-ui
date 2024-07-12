package main

import (
	"bufio"
	"fmt"
	"net"
	"slices"
)

func main() {
	// check dante version
	// check dante service file
	// check dante config file
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
		atIndex := slices.Index(bytes[percentIndex:], '@')
		if atIndex < 0 {
			continue
		}
		username := bytes[percentIndex : percentIndex+atIndex]
		openParenIndex := slices.Index(bytes[atIndex:], '(')
		if openParenIndex < 0 {
			continue
		}
		closeParenIndex := slices.Index(bytes[openParenIndex:], ')')
		if closeParenIndex < 0 {
			continue
		}
		count := bytes[percentIndex+atIndex+openParenIndex : percentIndex+atIndex+openParenIndex+closeParenIndex]
		fmt.Printf("%s -> %s bytes\n", username, count)
	}
}
