package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/hpcloud/tail"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed public/build/*
var publicFS embed.FS

type User struct {
	Username     string `json:"username"`
	TotalUsage   int64  `json:"totalUsage"`
	AllowedUsage int64  `json:"allowedUsage"`
	ExpiresAt    string `json:"expiresAt"`
	Disabled     bool   `json:"disabled"`
}

type Users struct {
	users map[string]*User
	mu    sync.RWMutex
	cache []string
}

func init() {
	// check dante version
	out, err := exec.Command("danted", "-v").Output()
	if err != nil {
		panic(err)
	}
	if !strings.Contains(string(out), "v1.4.2.") {
		fmt.Println("Please use dante v1.4.2. for best compatibility")
		fmt.Println("Current dante version: " + string(out))
	}

	if _, err := os.Stat("db.json"); errors.Is(err, os.ErrNotExist) {
		f, _ := os.Create("db.json")
		f.WriteString("{}")
	}
}

func main() {
	users := Users{}
	users.users = make(map[string]*User)

	// open database
	execPath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	db, err := badger.Open(badger.DefaultOptions(filepath.Join(filepath.Dir(execPath), "badger")))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// restart dante for correct usage tracking
	_, err = exec.Command("systemctl", "restart", "danted").CombinedOutput()
	if err != nil {
		panic(err)
	}
	log.Println("danted server restarted")

	// get group id of danteuisucks
findGroupID:
	groupID := ""
	bytes, err := os.ReadFile("/etc/group")
	if err != nil {
		panic(err)
	}
	for _, l := range strings.Split(string(bytes), "\n") {
		if strings.Contains(l, "danteuisucks") {
			// danteuisucks:x:groupID:
			parts := strings.Split(l, ":")
			groupID = parts[len(parts)-2]
		}
	}
	if groupID == "" {
		log.Print("danteuisucks group not found, creating group...")
		_, err := exec.Command("groupadd", "danteuisucks").CombinedOutput()
		if err != nil {
			panic(err)
		}
		goto findGroupID
	} else {
		log.Println("danteuisucks group id: " + groupID)
	}

	// find active users in /etc/passwd
	bytes, err = os.ReadFile("/etc/passwd")
	if err != nil {
		panic(err)
	}
	for _, l := range strings.Split(string(bytes), "\n") {
		if strings.Contains(l, "/home/") && strings.Contains(l, "/bin/false") {
			// username:x:groupID:groupID::/home/username:/bin/false
			username := strings.Split(l, ":")[0]
			log.Printf("Found [%s] in /etc/passwd\n", username)
			users.cache = append(users.cache, username)
			users.users[username] = &User{Username: username}
			if err = db.Update(func(txn *badger.Txn) error {
				item, err := txn.Get([]byte(username))
				if err != nil {
					if err == badger.ErrKeyNotFound {
						return db.Update(func(txn *badger.Txn) error { return txn.Set([]byte(username), []byte(fmt.Sprint(0))) })
					}
					return err
				}
				return item.Value(func(val []byte) error {
					if err != nil {
						return err
					}
					previousCount, err := strconv.ParseInt(string(val), 10, 64)
					if err != nil {
						return err
					}
					users.users[username].TotalUsage += previousCount
					return nil
				})
			}); err != nil {
				panic(err)
			}
			chageLines, err := exec.Command("chage", "-l", "-i", username).CombinedOutput()
			if err != nil {
				panic(err)
			}
			for _, ll := range strings.Split(string(chageLines), "\n") {
				if strings.Contains(ll, "Account expires") {
					parts := strings.Split(ll, ":")
					date := strings.TrimSpace(parts[len(parts)-1])
					if date == "never" {
						users.users[username].ExpiresAt = "never"
						log.Printf("[%s] will never expire\n", username)
					} else {
						users.users[username].ExpiresAt = date
					}
				}
			}
		}
	}

	// update user usages in local db
	go func() {
		for {
			for _, u := range users.users {
				users.mu.RLock()
				if err = db.Update(func(txn *badger.Txn) error { return txn.Set([]byte(u.Username), []byte(fmt.Sprint(u.TotalUsage))) }); err != nil {
					panic(err)
				}
				users.mu.RUnlock()
			}
			time.Sleep(time.Second * 30)
		}
	}()

	// start http server
	go func() {
		e := echo.New()

		e.Use(middleware.CORS())

		if len(os.Args) > 1 && slices.Contains(os.Args, "--live") {
			log.Print("using live mode")
			execPath, err := os.Executable()
			if err != nil {
				panic(err)
			}
			path := filepath.Dir(execPath)
			e.Static("/", filepath.Join(path, "public", "build"))
		} else {
			log.Print("using embed mode")
			e.StaticFS("/*", echo.MustSubFS(publicFS, "public/build"))
		}

		e.GET("/api/users", func(c echo.Context) error {
			users.mu.RLock()
			defer users.mu.RUnlock()
			return c.JSON(200, users.users)
		})

		e.POST("/api/users", func(c echo.Context) error {
			var data struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			err := json.NewDecoder(c.Request().Body).Decode(&data)
			if err != nil {
				return c.String(400, err.Error())
			}
			passwordBytes, err := exec.Command("openssl", "passwd", "-6", "-salt", "xyz", data.Password).CombinedOutput()
			if err != nil {
				log.Println(err)
				return c.NoContent(500)
			}
			expiresAt := time.Now().Add(time.Hour * 24 * 30).Format("2006-01-02")
			bytes, err := exec.Command("useradd", "-s", "/bin/false", "-e", expiresAt, "-p", strings.TrimSpace(string(passwordBytes)), data.Username).CombinedOutput()
			if err != nil {
				if strings.Contains(string(bytes), "already exists") {
					return c.NoContent(409)
				}
				log.Println(err, string(bytes))
				return c.NoContent(500)
			}
			users.mu.Lock()
			users.cache = append(users.cache, data.Username)
			if err = db.Update(func(txn *badger.Txn) error { return txn.Set([]byte(data.Username), []byte(fmt.Sprint(0))) }); err != nil {
				panic(err)
			}
			users.users[data.Username] = &User{Username: data.Username, ExpiresAt: expiresAt}
			users.mu.Unlock()
			return c.NoContent(201)
		})

		e.DELETE("/api/users", func(c echo.Context) error {
			var data struct {
				Username string `json:"username"`
			}
			err := json.NewDecoder(c.Request().Body).Decode(&data)
			if err != nil {
				return c.String(400, err.Error())
			}
			bytes, err := exec.Command("userdel", data.Username).CombinedOutput()
			if err != nil {
				if strings.Contains(string(bytes), "does not exist") {
					return c.NoContent(400)
				}
				log.Println(err, string(bytes))
				return c.NoContent(500)
			}
			users.mu.Lock()
			users.cache = slices.DeleteFunc(users.cache, func(s string) bool {
				return s == data.Username
			})
			delete(users.users, data.Username)
			users.mu.Unlock()
			return c.NoContent(200)
		})

		e.Logger.Fatal(e.Start(":10800"))
	}()

	// read and parse dante log file
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
		if !slices.Contains(users.cache, username) {
			continue
		}
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
		if user, ok := users.users[username]; ok {
			user.TotalUsage += countInt
		} else {
			log.Printf("failed to find [%s] in users map\n", username)
		}
		users.mu.Unlock()
	}
}
