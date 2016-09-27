package main

import (
	"fmt"
	"github.com/labstack/echo"
	"github.com/labstack/echo/engine/standard"
	"github.com/labstack/echo/middleware"
	"github.com/oschwald/geoip2-golang"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

var db *geoip2.Reader
var mutex = &sync.Mutex{}

type geoData struct {
	City      string      `json:"city"`
	Country   string      `json:"country"`
	IsoCode   string      `json:"country_code"`
	Latitude  float64     `json:"latitude"`
	Longitude float64     `json:"longitude"`
	TimeZone  string      `json:"time_zone"`
	IP        string      `json:"ip"`
	Status    string      `json:"status"`
	Err       *geoDataErr `json:"error"`
}

type geoDataErr struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	IP      string `json:"ip"`
}

type localChanData struct {
	ip    string
	ipout chan *geoData
}

func main() {
	port := os.Getenv("PORT")
	mmdbPath := os.Getenv("MMDB_PATH")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	if mmdbPath == "" {
		mmdbPath = "GeoLite2-City.mmdb"
	}

	var openerr error
	db, openerr = geoip2.Open(mmdbPath)
	checkErr(openerr)

	go func() {
		for range time.Tick(24 * time.Hour) {
			curTime := time.Now().UTC()
			newfilepath := fmt.Sprintf("GeoLite2-City-%02d.mmdb", curTime.Month())
			firstTues := firstTuesday(curTime.Year(), curTime.Month())
			if firstTues == curTime.Day() {
				if _, err := os.Stat(newfilepath); err == nil {
					mutex.Lock()
					println("mutex locked")
					db.Close()
					//exists, so open the new one
					println("opening the new one")
					db, openerr = geoip2.Open(newfilepath)
					mutex.Unlock()
					println("mutex unlocked")
					checkErr(openerr)
				}
			}
		}
	}()

	ipin := make(chan localChanData)
	defer close(ipin)

	for i := 0; i < 300; i++ {
		go func(ipin chan localChanData) {
			for input := range ipin {
				record := queryDB(input.ip)
				input.ipout <- record
			}
		}(ipin)
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Gzip())
	e.Use(middleware.Recover())

	e.GET("/json", func(c echo.Context) error {
		passedIP := c.QueryParam("ip")

		var clientIP string
		if passedIP != "" {
			clientIP = passedIP
		} else {
			clientIP = c.Request().RealIP()
		}
		data := make(chan *geoData)
		defer close(data)
		ipin <- localChanData{clientIP, data}
		for output := range data {
			if output.Err != nil {
				return c.JSON(http.StatusNotFound, output.Err)
			}

			return c.JSON(http.StatusOK, output)
		}

		return nil
	})

	e.Logger().Debug("Starting on port " + port)
	e.Run(standard.New(":" + port))
}

func queryDB(strip string) *geoData {
	ip := net.ParseIP(strip)
	record, err := db.City(ip)
	if err != nil {
		var data geoData
		var geoerr geoDataErr
		geoerr.IP = strip
		geoerr.Message = err.Error()
		geoerr.Status = "fail"
		data.Err = &geoerr

		return &data
	}

	return &geoData{record.City.Names["en"], record.Country.Names["en"], record.Country.IsoCode, record.Location.Latitude, record.Location.Longitude, record.Location.TimeZone, strip, "success", nil}
}

// firstTuesday returns the day of the first Tuesday in the given month.
func firstTuesday(year int, month time.Month) int {
	t := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)

	return (8-int(t.Weekday()))%7 + 2
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
