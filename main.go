package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//Configuration - Connection and Record data taken from config.json
type Configuration struct {
	AuthEmail      string `json:"authEmail"`
	AuthKey        string `json:"authKey"`
	ZoneIdentifier string `json:"zoneIdentifier"`
	RecordName     string `json:"recordName"`
	EnableProxy    bool   `json:"proxy"`
}

//DNSUpdateRequest - Request sent to update A record to Cloud Flare
//"{\"id\":\"$zone_identifier\",\"type\":\"A\",\"proxied\":${proxy},\"name\":\"$record_name\",\"content\":\"$ip\"})
type DNSUpdateRequest struct {
	ZoneIdentifier string `json:"id"`
	RecordType     string `json:"type"`
	EnableProxy    bool   `json:"proxied"`
	RecordName     string `json:"name"`
	IPAddress      string `json:"content"`
	TTL            int16  `json:"ttl"`
}

//getCurrentIP - Gets the current Public IPv4 address from ipv4.icanhazip.com
func getCurrentIP() (string, error) {

	resp, err := http.Get("https://ipv4.icanhazip.com/")

	if err != nil {
		log.Printf("error when getting current ip :- %s", err.Error())
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Printf("error when getting current ip :- %s", err.Error())
		return "", err
	}

	return string(body), nil
}

//getPreviousIP - gets the old IP which was previously set from a text file.
//This way we dont have to make a unnecessary request to clould flare.
func getPreviousIP() (string, error) {
	file, err := os.Open("oldip.txt")
	if err != nil {
		log.Printf("error when getting previous ip :- %s", err.Error())
		return "", err
	}

	defer file.Close()

	ipBytes, err := ioutil.ReadAll(file)

	if err != nil {
		log.Printf("error when getting previous ip :- %s", err.Error())
		return "", err
	}

	return string(ipBytes), nil
}

//getRecordIdentifier - Get Record Identifier from Cloudflare
func getRecordIdentifier(configuration *Configuration) (string, error) {
	//"https://api.cloudflare.com/client/v4/zones/$zone_identifier/dns_records?name=$record_name" -H "X-Auth-Email: $auth_email" -H "X-Auth-Key: $auth_key" -H "Content-Type: application/json"
	request, err := http.NewRequest("GET", fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?name=%s", configuration.ZoneIdentifier, configuration.RecordName), nil)
	if err != nil {
		log.Printf("error when getting dns record identifier :- %s", err.Error())
		return "", err
	}

	request.Header.Add("X-Auth-Email", configuration.AuthEmail)
	request.Header.Add("X-Auth-Key", configuration.AuthKey)
	request.Header.Add("Content-Type", "application/json")

	client := http.DefaultClient

	resp, err := client.Do(request)
	if err != nil {
		log.Printf("error when getting dns record identifier :- %s", err.Error())
		return "", err
	}

	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var responseJSON map[string]interface{}
	err = decoder.Decode(&responseJSON)

	if err != nil {
		log.Printf("error when getting dns record identifier :- %s", err.Error())
		return "", err
	}

	if responseJSON["success"].(bool) {
		var result = responseJSON["result"].([]interface{})
		if len(result) > 0 {
			var recordMap = result[0].(map[string]interface{})
			return recordMap["id"].(string), nil
		}
		return "", errors.New("error when getting dns record identifier :- server returned empty result")

	}

	return "", errors.New("error when getting dns record identifier :- server returned error")
}

//updateCurrentIPToDNS - Updates current IP to cloudflare dns A record
func updateCurrentIPToDNS(configuration *Configuration, currentIP string, dnsIdentifier string) error {

	//create request body
	var dNSUpdateRequest = DNSUpdateRequest{
		IPAddress:      currentIP,
		EnableProxy:    configuration.EnableProxy,
		RecordName:     configuration.RecordName,
		RecordType:     "A",
		ZoneIdentifier: configuration.ZoneIdentifier,
		TTL:            120,
	}
	dNSUpdateRequestJSON, err := json.Marshal(dNSUpdateRequest)

	if err != nil {
		log.Printf("error when updating dns record :- %s", err.Error())
		return err
	}

	request, err := http.NewRequest("PUT", fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
		configuration.ZoneIdentifier,
		dnsIdentifier),
		bytes.NewBuffer(dNSUpdateRequestJSON))
	if err != nil {
		log.Printf("error when updating dns record :- %s", err.Error())
		return err
	}

	//add header
	request.Header.Add("X-Auth-Email", configuration.AuthEmail)
	request.Header.Add("X-Auth-Key", configuration.AuthKey)
	request.Header.Add("Content-Type", "application/json")

	client := http.DefaultClient
	resp, err := client.Do(request)
	if err != nil {
		log.Printf("error when updating dns record :- %s", err.Error())
		return err
	}

	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var responseJSON map[string]interface{}
	err = decoder.Decode(&responseJSON)

	if err != nil {
		log.Printf("error when getting dns record identifier :- %s", err.Error())
		return err
	}

	if !responseJSON["success"].(bool) {
		return fmt.Errorf("error when updating dns record %v", responseJSON)
	}

	return nil
}

func init() {

	// log to console and file
	f, err := os.OpenFile("ddns.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	wrt := io.MultiWriter(os.Stdout, f)

	log.SetOutput(wrt)
	log.SetPrefix("DDNS SCRIPT ")
	log.SetFlags(log.LstdFlags | log.Lshortfile) //Log Line Number to Debug errors
}

func doEvery(d time.Duration, f func(time.Time)) {
	for x := range time.Tick(d) {
		f(x)
	}
}

func checkAndUpdateDNS(configuration *Configuration) {
	var currentPublicIP string
	var previousPublicIP string
	var err error
	//get current ip address
	currentPublicIP, err = getCurrentIP()
	if err != nil {
		log.Fatalf("error when getting current ip :- %s", err.Error())
	}
	log.Printf("Current public ipv4 address :- %s", currentPublicIP)

	//get ip address previously set to cloudflare
	previousPublicIP, err = getPreviousIP()
	if err != nil {
		log.Fatalf("error when getting previous ip :- %s", err.Error())
	}
	log.Printf("Current previous ipv4 address :- %s", previousPublicIP)

	//compare both ip addresses
	if strings.Trim(previousPublicIP, "") != strings.Trim(currentPublicIP, "") {
		//get DNS record identifier
		var dnsRecordID string
		dnsRecordID, err = getRecordIdentifier(configuration)
		if err != nil {
			log.Fatalf("error when getting dns record identifier :- %s", err.Error())
		}
		log.Printf("dns record id : %s", dnsRecordID)

		//update ip address to dns
		err = updateCurrentIPToDNS(configuration, currentPublicIP, dnsRecordID)
		if err != nil {
			log.Fatalf("error when updating dns record :- %s", err.Error())
		}

		ipAddressBuffer := []byte(currentPublicIP)
		err := ioutil.WriteFile("oldip.txt", ipAddressBuffer, 0644)
		if err != nil {
			log.Fatalf("error when writing to oldip.txt :- %s", err.Error())
		}
	} else {
		log.Println("both current and previous ip addresses are the same, exiting...")
	}
}

func main() {
	log.Println("Starting DDNS Script")
	var configuration Configuration

	var err error

	//get configuration
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("error opening config.json :- %s", err.Error())
	}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&configuration)
	if err != nil {
		log.Fatalf("error decoding config.json :- %s", err.Error())
	}
	//Run every 5 mins
	ticker := time.NewTicker(300000 * time.Millisecond)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				checkAndUpdateDNS(&configuration)
			}
		}
	}()

	//Catch Sigterm Signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	for sig := range c {
		log.Println(sig.String())
		ticker.Stop()
		done <- true
		log.Println("Stopped")
		break
	}

	log.Println("Ending   DDNS Script")
}
