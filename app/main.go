package main

import (
	"encoding/json"
	"fmt"
	flags "github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ApiKey The API key is static, used for AWS
const ApiKey = "3_qRseYzrUJl1VyxvSJANalu_kNgQ83swB1B9uzgms58--5w1ClVNmrFdsDnWVQQCl"

// ApiRouteLogin The API route for the login, parameters are passed as query string
const ApiRouteLogin = "https://accounts.eu1.gigya.com/accounts.login"

const ApiRouteJwt = "https://accounts.eu1.gigya.com/accounts.getJWT"

const ApiRouteExecute = "https://hkgmr8v960.execute-api.eu-west-1.amazonaws.com/prod/c/login"

// ApiRouteDevices The API route for the devices
const ApiRouteDevices = "https://hkgmr8v960.execute-api.eu-west-1.amazonaws.com/prod/c/registered-devices"

// ApiRouteDeviceInfo The API route for the device info, placeholder is device name
const ApiRouteDeviceInfo = "https://hkgmr8v960.execute-api.eu-west-1.amazonaws.com/prod/c/%s/r/initial"

// DeviceInfoRequestPayload The payload for the device info request, both placeholders are device UUID
const DeviceInfoRequestPayload = `{"deviceconfigquery":[{"id":"%s","r":{"r":["sensors"]}}],"includestates":true,"eventsubscription":{"include":[{"filter":{"o":"= %s"}}]}}`

var (
	fanspeed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_fanspeed",
			Help: "Fanspeed (%)",
		},
		[]string{"sensor"},
	)

	temperature = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_temp",
			Help: "Temperature (C)",
		},
		[]string{"sensor"},
	)

	humidity = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_humid",
			Help: "Relative humidity (%)",
		},
		[]string{"sensor"},
	)

	voc = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_voc",
			Help: "Volatile organic compounds (ppb)",
		},
		[]string{"sensor"},
	)

	pm25 = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_pm25",
			Help: "Particulate matter, 2.5 micron (ug/m^3)",
		},
		[]string{"sensor"},
	)

	pm10 = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_pm10",
			Help: "Particulate matter, 10 micron (ug/m^3)",
		},
		[]string{"sensor"},
	)

	pm1 = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blueair_pm1",
			Help: "Particulate matter, 1 micron (ug/m^3)",
		},
		[]string{"sensor"},
	)
)

type SessionInfo struct {
	Token  string `json:"sessionToken"`
	Secret string `json:"sessionSecret"`
}

type OauthResponse struct {
	SessionInfo SessionInfo `json:"sessionInfo"`
}

type JwtData struct {
	Token string `json:"id_token"`
}

type AccessToken struct {
	Token string `json:"access_token"`
}

type Device struct {
	Uuid                string `json:"uuid"`
	Name                string `json:"name"`
	WifiFirmwareVersion string `json:"wifi-firmware"`
	McuFirmwareVersion  string `json:"mcu-firmware"`
	Type                string `json:"type"`
	Mac                 string `json:"mac"`
	UserType            string `json:"user-type"`
}

type Devices struct {
	Devices []Device `json:"devices"`
}

type DeviceInfoResponse struct {
	DeviceInfo []struct {
		Uuid          string `json:"id"`
		Configuration struct {
			DetailedInfo struct {
				Name string `json:"name"`
			} `json:"di"`
		} `json:"configuration"`
		SensorData []SensorDataItem `json:"sensordata"`
	} `json:"deviceInfo"`
}

type SensorDataItem struct {
	Value     string    `json:"v"`
	Name      string    `json:"n"`
	Timestamp time.Time `json:"t"`
}

type SensorData struct {
	Timestamp time.Time
	Voc       int
	Pm25      int
	Pm10      int
	Pm1       int
	Temp      int
	Humid     int
	FanSpeed  int
}

func (s *SensorDataItem) UnmarshalJSON(data []byte) error {
	type Alias SensorDataItem
	aux := &struct {
		*Alias
		Timestamp uint64 `json:"t"`
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.Timestamp = time.Unix(int64(aux.Timestamp), 0).In(time.UTC)
	return nil
}

func unmarshal(jsonBlob []byte) (*SensorData, string, error) {
	var jsonData DeviceInfoResponse
	if err := json.Unmarshal(jsonBlob, &jsonData); err != nil {
		return nil, "", err
	}

	var data SensorData

	for _, item := range jsonData.DeviceInfo[0].SensorData {
		itemValue, _ := strconv.Atoi(item.Value) // assuming conversion always succeeds
		switch item.Name {
		case "tVOC":
			data.Voc = itemValue
		case "h":
			data.Humid = itemValue
		case "t":
			data.Temp = itemValue
		case "pm10":
			data.Pm10 = itemValue
		case "pm2_5":
			data.Pm25 = itemValue
		case "pm1":
			data.Pm1 = itemValue
		case "fsp0":
			data.FanSpeed = itemValue
		}
		data.Timestamp = item.Timestamp
	}
	return &data, jsonData.DeviceInfo[0].Configuration.DetailedInfo.Name, nil
}

func recordMetricsLoop(devices []Device, delay time.Duration, token AccessToken, tokenExpiration time.Time, email string, password string) {
	for {
		for _, device := range devices {
			go recordMetricsForSensor(device, token)
		}

		time.Sleep(delay)

		if time.Now().After(tokenExpiration) {
			log.Print("Token has expired, refreshing...")
			tokenExpiration = time.Now().Add(24 * time.Hour)
			token = login(email, password)
			log.Printf("Logged in successfully: %s", email)
			log.Printf("Token expires at: %s", tokenExpiration.Format(time.RFC3339))
		}
	}
}

func recordMetricsForSensor(device Device, token AccessToken) {
	sensorData, name := sensorData(token, device)

	temperature.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.Temp))
	humidity.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.Humid))
	voc.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.Voc))
	pm25.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.Pm25))
	pm10.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.Pm10))
	pm1.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.Pm1))
	fanspeed.With(prometheus.Labels{"sensor": name}).Set(float64(sensorData.FanSpeed))
}

func doOauth(email string, password string) OauthResponse {
	client := http.Client{Timeout: 10 * time.Second}

	data := url.Values{}
	data.Set("apikey", ApiKey)
	data.Set("loginID", email)
	data.Set("password", password)
	data.Set("targetEnv", "mobile")

	req, err := http.NewRequest("POST", ApiRouteLogin, strings.NewReader(data.Encode()))
	req.Header.Add("User-Agent", "Blueair/58 CFNetwork/1327.0.4 Darwin/21.2.0")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Host", "accounts.eu1.gigya.com")
	req.Header.Add("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Unable to connect to Blueair API: %s", err)
		os.Exit(101)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body: %s", err)
		os.Exit(74)
	}

	var sessionInfo OauthResponse
	if err := json.Unmarshal(body, &sessionInfo); err != nil {
		log.Printf("Unable to unmarshal response body: %s", err)
		os.Exit(74)
	}

	return sessionInfo
}

func doJwt(email string, password string) JwtData {
	sessionInfo := doOauth(email, password)

	client := http.Client{Timeout: 10 * time.Second}

	data := url.Values{}
	data.Set("oauth_token", sessionInfo.SessionInfo.Token)
	data.Set("secret", sessionInfo.SessionInfo.Secret)
	data.Set("targetEnv", "mobile")

	req, err := http.NewRequest("POST", ApiRouteJwt, strings.NewReader(data.Encode()))
	req.Header.Add("User-Agent", "Blueair/58 CFNetwork/1327.0.4 Darwin/21.2.0")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Host", "accounts.eu1.gigya.com")
	req.Header.Add("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Unable to connect to Blueair API: %s", err)
		os.Exit(101)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body: %s", err)
		os.Exit(74)
	}

	var jwtResponse JwtData
	if err := json.Unmarshal(body, &jwtResponse); err != nil {
		log.Printf("Unable to unmarshal response body: %s", err)
		os.Exit(74)
	}

	return jwtResponse
}

func login(email string, password string) AccessToken {
	jwtResponse := doJwt(email, password)

	client := http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("POST", ApiRouteExecute, nil)
	req.Header.Add("User-Agent", "Blueair/58 CFNetwork/1327.0.4 Darwin/21.2.0")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Host", "hkgmr8v960.execute-api.eu-west-1.amazonaws.com")
	req.Header.Add("idtoken", jwtResponse.Token)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", jwtResponse.Token))

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Unable to connect to Blueair API: %s", err)
		os.Exit(101)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body: %s", err)
		os.Exit(74)
	}

	var authorizationToken AccessToken
	if err := json.Unmarshal(body, &authorizationToken); err != nil {
		log.Printf("Unable to unmarshal response body: %s", err)
		os.Exit(74)
	}

	return authorizationToken
}

func devices(token AccessToken) Devices {
	client := http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", ApiRouteDevices, nil)
	req.Header.Add("User-Agent", "Blueair/58 CFNetwork/1327.0.4 Darwin/21.2.0")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Host", "hkgmr8v960.execute-api.eu-west-1.amazonaws.com")
	req.Header.Add("idtoken", token.Token)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token.Token))

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Unable to connect to Blueair API: %s", err)
		os.Exit(101)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body: %s", err)
		os.Exit(74)
	}

	// Unmarshal json into Devices
	var devices Devices
	if err := json.Unmarshal(body, &devices); err != nil {
		log.Printf("Unable to unmarshal response body: %s", err)
		os.Exit(74)
	}

	return devices
}

func sensorData(token AccessToken, device Device) (*SensorData, string) {
	client := http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", fmt.Sprintf(ApiRouteDeviceInfo, device.Name),
		strings.NewReader(fmt.Sprintf(DeviceInfoRequestPayload, device.Uuid, device.Uuid)))
	req.Header.Add("User-Agent", "Blueair/58 CFNetwork/1327.0.4 Darwin/21.2.0")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Host", "hkgmr8v960.execute-api.eu-west-1.amazonaws.com")
	req.Header.Add("idtoken", token.Token)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token.Token))
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Unable to connect to Blueair API: %s", err)
		os.Exit(101)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body: %s", err)
		os.Exit(74)
	}

	sensorData, name, err := unmarshal(body)
	if err != nil {
		log.Printf("Failed to unmarshal the response body: %s", err)
		os.Exit(74)
	}

	return sensorData, name
}

func main() {
	var opts struct {
		Address  string        `short:"a" long:"address" default:"0.0.0.0:2735" description:"Address to listen on" env:"BLUEAIR_ADDRESS"`
		Delay    time.Duration `short:"d" long:"delay" default:"300s" description:"Delay between attempts to refresh metrics" env:"BLUEAIR_DELAY"`
		Email    string        `short:"e" long:"email" required:"true" description:"Email address for Blueair login" env:"BLUEAIR_EMAIL"`
		Password string        `short:"p" long:"password" required:"true" description:"Password for Blueair login" env:"BLUEAIR_PASSWORD"`
	}

	if _, err := flags.Parse(&opts); err != nil {
		// it only seems to return an error when `-h` /
		// `--help` is passed, and it already prints the help
		// text in that case, so there's no need to print the
		// message again.
		os.Exit(22)
	}

	tokenExpiration := time.Now().Add(24 * time.Hour)
	authorizationToken := login(opts.Email, opts.Password)
	log.Printf("Logged in successfully: %s", opts.Email)
	log.Printf("Token expires at: %s", tokenExpiration.Format(time.RFC3339))

	// loop devices and print all their properties
	devices := devices(authorizationToken)

	if len(devices.Devices) == 0 {
		log.Print("No devices found")
		os.Exit(1)
	}

	log.Printf("Found %d devices", len(devices.Devices))
	for _, device := range devices.Devices {
		log.Print("--------------------------------------------------")
		log.Printf("Device UUID: %s", device.Uuid)
		log.Printf("Device Name: %s", device.Name)
		log.Printf("Device Wifi Firmware Version: %s", device.WifiFirmwareVersion)
		log.Printf("Device MCU Firmware Version: %s", device.McuFirmwareVersion)
		log.Printf("Device Type: %s", device.Type)
		log.Printf("Device Mac: %s", device.Mac)
		log.Printf("Device User Type: %s", device.UserType)
	}
	log.Print("--------------------------------------------------")
	log.Print("Start recording sensor data...")

	go recordMetricsLoop(devices.Devices, opts.Delay, authorizationToken, tokenExpiration, opts.Email, opts.Password)

	http.Handle("/metrics", promhttp.Handler())

	log.Printf("listening on %v", opts.Address)
	log.Fatal(http.ListenAndServe(opts.Address, nil))
}
