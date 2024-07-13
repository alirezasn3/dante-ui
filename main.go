package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/hpcloud/tail"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Users struct {
	users map[string]int64
	mu    sync.RWMutex
}

func main() {
	// check dante version
	out, err := exec.Command("danted", "-v").Output()
	if err != nil {
		panic(err)
	}
	if !strings.Contains(string(out), "v1.4.2.") {
		fmt.Println("Please use dante v1.4.2. for best compatibility")
		fmt.Println("Current dante version: " + string(out))
	}

	users := Users{}
	users.users = make(map[string]int64)

	go func() {
		e := echo.New()

		e.Use(middleware.CORS())

		// handle static files
		execPath, err := os.Executable()
		if err != nil {
			panic(err)
		}
		path := filepath.Dir(execPath)
		e.Static("/", filepath.Join(path, "public", "build"))

		e.GET("/api/users", func(c echo.Context) error {
			users.mu.RLock()
			defer users.mu.RUnlock()
			return c.JSON(200, users.users)
		})

		e.Logger.Fatal(e.Start(":10800"))
	}()

	t, err := tail.TailFile("/var/log/danted.log", tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}

	for line := range t.Lines {
		percentIndex := strings.Index(line.Text, "%")
		if percentIndex < 0 {
			continue
		}
		atIndex := strings.Index(line.Text[percentIndex:], "@") + percentIndex
		if atIndex < 0 {
			continue
		}
		username := line.Text[percentIndex+1 : atIndex]
		openParenIndex := strings.Index(line.Text[atIndex:], "(") + atIndex
		if openParenIndex < 0 {
			continue
		}
		closeParenIndex := strings.Index(line.Text[openParenIndex:], ")") + openParenIndex
		if closeParenIndex < 0 {
			continue
		}
		count := line.Text[openParenIndex+1 : closeParenIndex]
		countInt, err := strconv.ParseInt(count, 10, 64)
		if err != nil {
			panic(err)
		}
		users.mu.Lock()
		users.users[username] += countInt
		users.mu.Unlock()
	}
}
