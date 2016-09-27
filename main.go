package main

import (
	"compress/gzip"
	"fmt"
	"github.com/labstack/echo"
	"github.com/labstack/echo/engine/standard"
	"github.com/labstack/echo/middleware"
	"github.com/oschwald/geoip2-golang"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const mmdburl = "http://geolite.maxmind.com/download/geoip/database/GeoLite2-City.mmdb.gz"

var db *geoip2.Reader
var fileDownloadCount = 0
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
	var serverETag string

	headresp, headerr := http.Head(mmdburl)
	if headerr != nil {
		println(headerr.Error())
	} else {
		serverETag = headresp.Header.Get("ETag")
	}

	var openerr error
	db, openerr = geoip2.Open(mmdbPath)
	checkErr(openerr)

	go func() {
		for range time.Tick(24 * time.Hour) {
			newfilepath := fmt.Sprintf("GeoLite2-City-%d.mmdb", fileDownloadCount)

			resp, headerr := http.Head(mmdburl)
			if headerr != nil {
				continue
			}

			ETag := resp.Header.Get("Last-Modified")
			lastModified, perr := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))
			if perr != nil {
				println(perr.Error())
				continue
			}

			if ETag != serverETag || lastModified.After(time.Now()) {
				out, fileerr := os.Create(newfilepath)
				if fileerr != nil {
					println(fileerr.Error())
				}
				defer out.Close()

				resp, downloaderr := http.Get(mmdburl)
				if downloaderr != nil {
					println(downloaderr.Error())
				}

				defer resp.Body.Close()

				r, unziperr := gzip.NewReader(resp.Body)
				defer r.Close()

				io.Copy(out, r)

				if unziperr != nil {
					println("downloaded the new file")
				}

				_, copyerr := io.Copy(out, resp.Body)
				if copyerr != nil {
					println(copyerr.Error())
					continue
				}

				mutex.Lock()

				db.Close()
				//exists, so open the new one
				println("opening the new one")

				db, openerr = geoip2.Open(newfilepath)

				mutex.Unlock()

				if openerr != nil {
					mutex.Lock()

					println("opening the old one")

					var retryErr error
					db, retryErr = geoip2.Open(mmdbPath)
					checkErr(retryErr)

					mutex.Unlock()

				} else {
					os.Remove(mmdbPath)
					mmdbPath = newfilepath
					serverETag = ETag
				}

				mutex.Lock()
				fileDownloadCount++
				mutex.Unlock()
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

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
