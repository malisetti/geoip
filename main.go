package main

import (
	"github.com/oschwald/geoip2-golang"
	"log"
	"net"
	"os"

	"github.com/gin-gonic/gin"
)

type GeoData struct {
	City      string  `json:"city"`
	Country   string  `json:"country"`
	IsoCode   string  `json:"country_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	TimeZone  string  `json:"time_zone"`
	IP        string  `json:"ip"`
	Status    string  `json:"status"`
}

type GeoDataErr struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	IP string `json:"ip"`
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	db, err := geoip2.Open("GeoLite2-City.mmdb")
	defer db.Close()

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	router.GET("/ip/:ip", func(c *gin.Context) {
		var successResponce GeoData
		var errResponse GeoDataErr

		if err != nil {
			errResponse.Message = err.Error()
			errResponse.Status = "fail"
			errResponse.IP = ""
			c.JSON(500, errResponse)
			return
		}

		passedIP := c.Param("ip")

		var clientIP string
		if passedIP != "" {
			clientIP = passedIP
		} else {
			clientIP = c.ClientIP()
		}

		ip := net.ParseIP(clientIP)
		record, err := db.City(ip)
		if err != nil {
			errResponse.Message = err.Error()
			errResponse.Status = "fail"
			errResponse.IP = clientIP
			c.JSON(500, errResponse)
			return
		}
		successResponce.City = record.City.Names["en"]
		successResponce.Country = record.Country.Names["en"]
		successResponce.IsoCode = record.Country.IsoCode
		successResponce.Latitude = record.Location.Latitude
		successResponce.Longitude = record.Location.Longitude
		successResponce.TimeZone = record.Location.TimeZone
		successResponce.IP = clientIP
		successResponce.Status = "success"

		c.JSON(200, successResponce)
	})

	router.Run(":" + port)
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
