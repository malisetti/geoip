package main

import (
	"fmt"
	"github.com/labstack/echo"
	"github.com/labstack/echo/engine/fasthttp"
	"github.com/oschwald/geoip2-golang"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/middleware"
)

var db *geoip2.Reader

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
		for {
			time.Sleep(24 * time.Hour)

			curTime := time.Now().UTC()
			month := fmt.Sprintf("%02d", curTime.Month())
			newfilepath := "GeoLite2-City-" + month + ".mmdb"
			firstTues := firstTuesday(curTime.Year(), curTime.Month())
			if firstTues == curTime.Day() {
				if _, err := os.Stat(newfilepath); err == nil {
					db.Close()
					//exists, so open the new one
					println("opening the new one")
					db, openerr = geoip2.Open(newfilepath)
					checkErr(openerr)
				}
			}
		}
	}()

	ipin := make(chan localChanData)
	defer close(ipin)

	for i := 0; i < 300; i++ {
		go func(ipin chan localChanData) {
			for {
				select {
				case input := <-ipin:
					record := queryDB(input.ip)
					input.ipout <- record
				}
			}
		}(ipin)
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/ip/:ip", func(c echo.Context) error {
		passedIP := c.Param("ip")

		var clientIP string
		if passedIP != "" {
			clientIP = passedIP
		} else {
			clientIP = c.Request().RealIP()
		}
		data := make(chan *geoData)
		defer close(data)
		ipin <- localChanData{clientIP, data}
		for {
			select {
			case output := <-data:
				if output.Err != nil {
					return c.JSON(http.StatusNotFound, output.Err)
				}

				return c.JSON(http.StatusOK, output)
			}
		}
	})

	e.Logger().Debug("Starting on port " + port)
	e.Run(fasthttp.New(":" + port))
}

func queryDB(strip string) *geoData {
	ip := net.ParseIP(strip)
	record, err := db.City(ip)
	var data geoData
	if err != nil {
		var geoerr geoDataErr
		geoerr.IP = strip
		geoerr.Message = err.Error()
		geoerr.Status = "fail"
		data.Err = &geoerr

		return &data
	}
	data.City = record.City.Names["en"]
	data.Country = record.Country.Names["en"]
	data.IsoCode = record.Country.IsoCode
	data.Latitude = record.Location.Latitude
	data.Longitude = record.Location.Longitude
	data.TimeZone = record.Location.TimeZone
	data.IP = strip
	data.Status = "success"

	return &data
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
