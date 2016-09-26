package main

import (
	"github.com/labstack/echo"
	"github.com/labstack/echo/engine/fasthttp"
	"github.com/oschwald/geoip2-golang"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/labstack/echo/middleware"
)

type geoData struct {
	City      string  `json:"city"`
	Country   string  `json:"country"`
	IsoCode   string  `json:"country_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	TimeZone  string  `json:"time_zone"`
	IP        string  `json:"ip"`
	Status    string  `json:"status"`
}

type geoDataErr struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	IP      string `json:"ip"`
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

	db, err := geoip2.Open(mmdbPath)
	defer db.Close()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/ip/:ip", func(c echo.Context) error {
		var successResponce geoData
		var errResponse geoDataErr

		if err != nil {
			errResponse.Message = err.Error()
			errResponse.Status = "fail"
			errResponse.IP = ""
			return c.JSON(http.StatusInternalServerError, errResponse)
		}

		passedIP := c.Param("ip")

		var clientIP string
		if passedIP != "" {
			clientIP = passedIP
		} else {
			clientIP = c.Request().RealIP()
		}

		ip := net.ParseIP(clientIP)
		record, err := db.City(ip)
		if err != nil {
			errResponse.Message = err.Error()
			errResponse.Status = "fail"
			errResponse.IP = clientIP
			return c.JSON(http.StatusNotFound, errResponse)
		}
		successResponce.City = record.City.Names["en"]
		successResponce.Country = record.Country.Names["en"]
		successResponce.IsoCode = record.Country.IsoCode
		successResponce.Latitude = record.Location.Latitude
		successResponce.Longitude = record.Location.Longitude
		successResponce.TimeZone = record.Location.TimeZone
		successResponce.IP = clientIP
		successResponce.Status = "success"

		return c.JSON(http.StatusOK, successResponce)
	})

	e.Run(fasthttp.New(":" + port))
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
