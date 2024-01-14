package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type CloudflareConfig struct {
	ApiKey  string `yaml:"api-key"`
	Records []struct {
		Schedule string `yaml:"schedule"`
		Name     string `yaml:"name"`
		Id       string `yaml:"id"`
		ZoneID   string `yaml:"zone-id"`
	}
}

type Config struct {
	Cloudflare CloudflareConfig
	IpProvider string `yaml:"ip-provider"`
}

func forever() {
	for {
		time.Sleep(time.Hour)
	}
}

func startCron(config *Config, cron *cron.Cron, httpClient *http.Client) {

	// setup cron

	for _, record := range config.Cloudflare.Records {
		fmt.Print(record.Schedule)
		cron.AddFunc(record.Schedule, func() {
			fmt.Println("updating", record.Name)
			// update record
			updateRecord(config, httpClient, record.ZoneID, record.Id, record.Name)
		})
	}
	fmt.Printf("%+v\n", config)

	fmt.Printf("Cron started at %v+\n", time.Now())

	cron.Start()
	fmt.Printf("%+v\n", cron.Entries())
}

func printHelp() {
	fmt.Println("ddns help")
	fmt.Println("ddns ip")
	fmt.Println("ddns list zones")
	fmt.Println("ddns list records [zoneID]")
	fmt.Println("ddns list jobs")
}

func main() {

	// Read config file
	config := getConfig()

	// setup cron
	cron := cron.New()

	// setup HTTP client
	client := &http.Client{}

	// read args
	args := os.Args[1:]

	if len(args) == 0 {
		printHelp()
		return
	}

	if args[0] == "ip" {
		ip := getIp(config)
		fmt.Println(ip)
	} else if args[0] == "help" {

	} else if args[0] == "start" {
		startCron(config, cron, client)

		go forever()
		select {}
	} else if args[0] == "list" {

		if len(args) < 2 {
			fmt.Println("Specify resources to list, available resources are: zones, records")
			return

		}
		if args[1] == "zones" {
			// list zones
			fmt.Println("listing zones")
			zones, _ := listZones(config, client)
			fmt.Println(zones)

		} else if args[1] == "records" {
			if len(args) < 3 {
				fmt.Println("zone id required")
				return
			}
			// list records
			zoneId := args[2]
			fmt.Println("listing records for zone", zoneId)
			records, _ := listRecords(config, client, zoneId)
			fmt.Println(records)
		} else if args[1] == "jobs" {
			// list cron jobs
			fmt.Printf("%+v\n", cron.Entries())
		} else {
			fmt.Println("Specify resources to list, available resources are: zones, records")
		}
	} else {
		fmt.Println("Unknown commands, run `ddns help` for help")

	}
	// ip := getIp()
	// fmt.Println(ip)
}

func updateRecord(config *Config, client *http.Client, zoneId string, recordId string, name string) (bool, error) {
	ip := getIp(config)
	fmt.Println("Updating record" + recordId)
	jsonData, err := json.Marshal(CloudflarePatchDNSBody{
		Content: ip,
		Name:    name,
	})
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/zones/"+zoneId+"/dns_records/"+recordId, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		return false, err
	}
	req.Header.Add("Authorization", "Bearer "+config.Cloudflare.ApiKey)

	response, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending HTTP request:", err)
		return false, err
	}
	defer response.Body.Close()

	result := CloudflarePatchDNSResponse{}

	err = json.NewDecoder(response.Body).Decode(&result)
	if err != nil {
		fmt.Println("Error decoding JSON response:", err)
		return false, err
	}

	return true, nil

}

func getConfig() *Config {
	config := Config{}

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	data, err := os.ReadFile(path.Join(pwd, "config.yaml"))
	if err != nil {
		fmt.Println("Error reading config file, please create config.yaml")
		panic(err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("error: %v", err)
		panic(err)
	}

	return &config
}

func listZones(config *Config, httpc *http.Client) ([]CloudflareZone, error) {
	req, err := http.NewRequest("GET", "https://api.cloudflare.com/client/v4/zones", nil)
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+config.Cloudflare.ApiKey)
	client := httpc

	response, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending HTTP request:", err)
		return nil, err
	}
	defer response.Body.Close()

	result := CloudflareZoneResponse{}

	json.NewDecoder(response.Body).Decode(&result)

	zones := []CloudflareZone{}

	for index, value := range result.Result {
		zones = append(zones, CloudflareZone{ZoneID: value.ID, Name: value.Name})
		fmt.Println(index, value.Name, value.ID)
	}
	return zones, nil
}

func listRecords(config *Config, httpc *http.Client, zoneId string) ([]CloudflareRecords, error) {
	req, err := http.NewRequest("GET", "https://api.cloudflare.com/client/v4/zones/"+zoneId+"/dns_records", nil)
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+config.Cloudflare.ApiKey)

	response, err := httpc.Do(req)
	if err != nil {
		fmt.Println("Error sending HTTP request:", err)
		return nil, err
	}
	defer response.Body.Close()

	result := CloudflareRecordsResponse{}

	json.NewDecoder(response.Body).Decode(&result)

	records := []CloudflareRecords{}

	for index, value := range result.Result {
		records = append(records, CloudflareRecords{RecordID: value.ID, Name: value.Name, Content: value.Content})
		fmt.Println(index, value.Name, value.ID)
	}
	return records, nil
}

func getIp(config *Config) string {
	resp, err := http.Get(config.IpProvider)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	return strings.TrimSpace(string(body))
}
