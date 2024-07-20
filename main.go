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
	Password     string `json:"password"`
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

type Config struct {
	ListenAddress string `json:"listenAddress"`
	PublicAddress string `json:"publicAddress"`
}

func (u *Users) GetUser(username string) (*User, error) {
	user := &User{}
	err := u.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(username))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			err := json.Unmarshal(val, user)
			if err != nil {
				return err
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (u *Users) SetUser(user *User) error {
	return u.db.Update(func(txn *badger.Txn) error {
		bytes, err := json.Marshal(user)
		if err != nil {
			return err
		}
		return txn.Set([]byte(user.Username), bytes)
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

	// load config file
	var config Config
	bytes, err := os.ReadFile(filepath.Join(filepath.Dir(execPath), "config.json"))
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		panic(err)
	}
	log.Println("Loaded config from " + filepath.Join(filepath.Dir(execPath), "config.json"))

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
					user = &User{Username: username, ExpiresAt: time.Now().Add(time.Hour * 24 * 30).Unix(), AllowedUsage: 50 * 1024000000}
					e := users.SetUser(user)
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
				if e = users.SetUser(u); e != nil {
					panic(e)
				}
				if (u.ExpiresAt <= t || u.TotalUsage >= u.AllowedUsage) && !u.Locked {
					// lock user
					_, e = exec.Command("passwd", "-q", "-l", u.Username).CombinedOutput()
					if e != nil {
						log.Printf("Failed to lock [%s]\n", u.Username)
						panic(e)
					} else {
						log.Printf("Locked [%s]\n", u.Username)
					}
					lockStatusChanged = true
				} else if u.Locked && (u.ExpiresAt > t && u.TotalUsage < u.AllowedUsage) {
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
					lockStatusChanged = false
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

		e.GET("/api/public-address", func(c echo.Context) error {
			return c.String(200, config.PublicAddress)
		})

		e.GET("/api/users", func(c echo.Context) error {
			users.mu.RLock()
			defer users.mu.RUnlock()
			return c.JSON(200, users.users)
		})

		e.GET("/api/users/:username", func(c echo.Context) error {
			username := c.Param("username")
			users.mu.RLock()
			defer users.mu.RUnlock()
			if user, ok := users.users[username]; ok {
				return c.JSON(200, user)
			} else {
				return c.NoContent(404)
			}
		})

		e.POST("/api/users", func(c echo.Context) error {
			user := &User{}
			err := json.NewDecoder(c.Request().Body).Decode(user)
			if err != nil {
				return c.String(400, err.Error())
			}
			passwordBytes, err := exec.Command("openssl", "passwd", "-6", "-salt", "xyz", user.Password).CombinedOutput()
			if err != nil {
				log.Println(err)
				return c.NoContent(500)
			}
			bytes, err := exec.Command("useradd", "-r", "-s", "/bin/false", "-p", strings.TrimSpace(string(passwordBytes)), user.Username).CombinedOutput()
			if err != nil {
				if strings.Contains(string(bytes), "already exists") {
					return c.NoContent(409)
				}
				log.Println(err, string(bytes))
				return c.NoContent(500)
			}
			user.ExpiresAt = time.Now().Add(time.Hour * 24 * 30).Unix()
			user.AllowedUsage = 50 * 1024000000
			if err = users.SetUser(user); err != nil {
				panic(err)
			}
			users.mu.Lock()
			users.users[user.Username] = user
			users.cache = append(users.cache, user.Username)
			users.mu.Unlock()
			log.Printf("[%s] created\n", user.Username)
			return c.NoContent(201)
		})

		e.PATCH("/api/users", func(c echo.Context) error {
			var newUser map[string]interface{}
			err := json.NewDecoder(c.Request().Body).Decode(&newUser)
			if err != nil {
				return c.String(400, err.Error())
			}

			// check if a username is supplied
			if username, ok := newUser["username"]; ok {
				users.mu.Lock()
				defer users.mu.Unlock()
				// check if the user exists in map
				if oldUser, ok := users.users[username.(string)]; ok {
					// check if there is a new allowedUsage value
					if newAllowedUsage, ok := newUser["allowedUsage"]; ok {
						oldUser.AllowedUsage = newAllowedUsage.(int64)
					}
					// check if there is a new expiresAt value
					if newExpiresAt, ok := newUser["expiresAt"]; ok {
						oldUser.ExpiresAt = newExpiresAt.(int64)
					}
					// check if there is a new totalUsage value
					if totalUsageAt, ok := newUser["totalUsage"]; ok {
						oldUser.ExpiresAt = totalUsageAt.(int64)
					}
				} else {
					return c.NoContent(404)
				}
			} else {
				return c.NoContent(400)
			}

			return c.NoContent(200)
		})

		e.DELETE("/api/users", func(c echo.Context) error {
			user := &User{}
			err := json.NewDecoder(c.Request().Body).Decode(&user)
			if err != nil {
				return c.String(400, err.Error())
			}
			bytes, err := exec.Command("userdel", user.Username).CombinedOutput()
			if err != nil {
				if strings.Contains(string(bytes), "does not exist") {
					return c.NoContent(400)
				}
				log.Println(err, string(bytes))
				return c.NoContent(500)
			}
			if err = users.DeleteUser(user.Username); err != nil {
				panic(err)
			}
			users.mu.Lock()
			users.cache = slices.DeleteFunc(users.cache, func(s string) bool {
				return s == user.Username
			})
			delete(users.users, user.Username)
			users.mu.Unlock()
			log.Printf("[%s] removed", user.Username)
			return c.NoContent(200)
		})

		e.Logger.Fatal(e.Start(config.ListenAddress))
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
