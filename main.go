package main

import (
	"embed"
	"encoding/json"
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
	ExpiresAt    int64  `json:"expiresAt"`
	Locked       bool   `json:"locked"`
}

type Users struct {
	users map[string]*User
	mu    sync.RWMutex
	cache []string
	db    *badger.DB
}

func (u *Users) GetUser(username string) (*User, error) {
	user := &User{Username: username}
	err := u.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(username))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			parts := strings.Split(string(val), " ")
			totalUsage, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return err
			}
			expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return err
			}
			user.TotalUsage = totalUsage
			user.ExpiresAt = expiresAt
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (u *Users) SetUser(username string, totalUsage int64, expiresAt int64) error {
	return u.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(username), []byte(fmt.Sprintf("%d %d", totalUsage, expiresAt)))
	})
}

func (u *Users) DeleteUser(username string) error {
	return u.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(username))
	})
}

func init() {
	// check dante version
	out, err := exec.Command("danted", "-v").Output()
	if err != nil {
		log.Println("Dante is not installed")
		panic(err)
	}
	if !strings.Contains(string(out), "v1.4.2.") {
		fmt.Println("Please use dante v1.4.2. for best compatibility", "Current dante version: "+string(out))
	}
}

func main() {
	users := Users{}
	users.users = make(map[string]*User)

	// get executable path
	execPath, err := os.Executable()
	if err != nil {
		panic(err)
	}

	// open database
	db, err := badger.Open(badger.DefaultOptions(filepath.Join(filepath.Dir(execPath), "badger")))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	users.db = db

	// restart dante for correct usage tracking
	_, err = exec.Command("systemctl", "restart", "danted").CombinedOutput()
	if err != nil {
		log.Println("Failed to restart dante with systemctl")
		panic(err)
	}
	log.Println("Danted server restarted")

	// get group id of danteuisucks
findGroupID:
	groupID := ""
	etcGroupBytes, err := os.ReadFile("/etc/group")
	if err != nil {
		log.Println("Failed to open /etc/group")
		panic(err)
	}
	for _, l := range strings.Split(string(etcGroupBytes), "\n") {
		if strings.Contains(l, "danteuisucks") {
			// each line is in this format: danteuisucks:x:groupID:
			parts := strings.Split(l, ":")
			groupID = parts[len(parts)-2]
		}
	}
	if groupID == "" {
		log.Print("danteuisucks group not found, creating group...")
		_, err := exec.Command("groupadd", "danteuisucks").CombinedOutput()
		if err != nil {
			log.Println("Failed to create danteuisucks group with groupadd")
			panic(err)
		}
		goto findGroupID
	} else {
		log.Println("danteuisucks group id: " + groupID)
	}

	// find active users in /etc/passwd
	etcPasswdBytes, err := os.ReadFile("/etc/passwd")
	if err != nil {
		panic(err)
	}
	for _, l := range strings.Split(string(etcPasswdBytes), "\n") {
		if strings.Contains(l, "/home/") && strings.Contains(l, "/bin/false") {
			// each line is in this format: username:x:groupID:groupID::/home/username:/bin/false
			username := strings.Split(l, ":")[0]
			log.Printf("Found [%s] in /etc/passwd\n", username)

			// get user from database
			user, err := users.GetUser(username)
			if err != nil {
				if err == badger.ErrKeyNotFound {
					// create user on datebase if it doesn't exist
					e := users.SetUser(username, 0, time.Now().Add(time.Hour*24*30).Unix())
					if e != nil {
						panic(e)
					}
				} else {
					panic(err)
				}
			}
			users.users[username] = user

			// get lock status of each user
			passwdStatusBytes, err := exec.Command("passwd", "-S", username).CombinedOutput()
			if err != nil {
				log.Printf("Failed to get status of [%s]\n", username)
			}
			if strings.Contains(string(passwdStatusBytes), " P ") {
				user.Locked = false
			} else if strings.Contains(string(passwdStatusBytes), " L ") {
				user.Locked = true
			} else {
				panic("Failed to get lock status of [" + username + "]")
			}

			// add user to cache and map
			users.cache = append(users.cache, username)
			users.users[username] = &User{Username: username}
		}
	}

	// update user usages in local db
	go func() {
		var e error
		var t int64
		var lockStatusChanged bool
		for {
			t = time.Now().Unix()
			for _, u := range users.users {
				users.mu.RLock()
				if e = users.SetUser(u.Username, u.TotalUsage, u.ExpiresAt); e != nil {
					panic(e)
				}
				if u.ExpiresAt <= t && !u.Locked {
					// lock user
					_, e = exec.Command("passwd", "-q", "-l", u.Username).CombinedOutput()
					if e != nil {
						log.Printf("Failed to lock [%s]\n", u.Username)
						panic(e)
					} else {
						log.Printf("Locked [%s]\n", u.Username)
					}
					lockStatusChanged = true
				} else if u.Locked && u.ExpiresAt > t {
					// unlock user
					_, e = exec.Command("passwd", "-q", "-u", u.Username).CombinedOutput()
					if e != nil {
						log.Printf("Failed to unlock [%s]\n", u.Username)
						panic(e)
					} else {
						log.Printf("Unlocked [%s]\n", u.Username)
					}
					lockStatusChanged = true
				}
				users.mu.RUnlock()
				if lockStatusChanged {
					users.mu.Lock()
					u.Locked = !u.Locked
					users.mu.Unlock()
				}
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
			bytes, err := exec.Command("useradd", "-r", "-s", "/bin/false", "-p", strings.TrimSpace(string(passwordBytes)), data.Username).CombinedOutput()
			if err != nil {
				if strings.Contains(string(bytes), "already exists") {
					return c.NoContent(409)
				}
				log.Println(err, string(bytes))
				return c.NoContent(500)
			}
			expiresAt := time.Now().Add(time.Hour * 24 * 30).Unix()
			if err = users.SetUser(data.Username, 0, expiresAt); err != nil {
				panic(err)
			}
			users.mu.Lock()
			users.cache = append(users.cache, data.Username)
			users.users[data.Username] = &User{Username: data.Username, ExpiresAt: expiresAt}
			users.mu.Unlock()
			log.Printf("[%s] created\n", data.Username)
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
			if err = users.DeleteUser(data.Username); err != nil {
				panic(err)
			}
			users.mu.Lock()
			users.cache = slices.DeleteFunc(users.cache, func(s string) bool {
				return s == data.Username
			})
			delete(users.users, data.Username)
			users.mu.Unlock()
			log.Printf("[%s] removed", data.Username)
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
