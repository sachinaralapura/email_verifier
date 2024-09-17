package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/mcnijman/go-emailaddress"
)

type Email struct {
	address  string
	username string
	domain   string
}

func (e Email) String() string {
	if e.username == "" || e.domain == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", e.username, e.domain)
}

func (email *Email) parse() error {
	mail, err := emailaddress.Parse(email.address)
	if err != nil {
		email.domain = ""
		email.username = ""
		return err
	}
	email.username = mail.LocalPart
	email.domain = mail.Domain
	return nil
}

func getOutput(email Email) string {
	lineLength := max(len(email.address)+12, len(email.domain)+6, len(email.username)+9) + 5
	line := strings.Repeat("=", lineLength) + "\n"
	output := line
	output += email.String() + strings.Repeat("-", lineLength-(len(email.address)+12)) + "Valid Syntax\n"
	output += "Domain" + strings.Repeat("-", lineLength-(len(email.domain)+6)) + email.domain + "\n"
	output += "LocalPart" + strings.Repeat("-", lineLength-(len(email.username)+9)) + email.username + "\n"
	return output
}

func ParseEmailAddress(printChannel chan<- string, sem chan struct{}, wg *sync.WaitGroup) {
	emailAddresses := os.Args[2:]
	fmt.Println(emailAddresses)
	for _, emailAddress := range emailAddresses {
		wg.Add(1)

		go func(addr string) {
			defer wg.Done()
			sem <- struct{}{}

			email := new(Email)
			email.address = addr
			if err := email.parse(); err != nil {
				lineLength := max(len(email.address)+14, len(err.Error())) + 5
				output := strings.Repeat("=", lineLength) + "\n"
				output += email.address + strings.Repeat("-", lineLength-(len(email.address)+14)) + "Invalid Syntax\n"
				output += err.Error() + "\n"
				printChannel <- output
				<-sem
				return
			}
			output := getOutput(*email)
			printChannel <- output
			<-sem
		}(emailAddress)
	}
}

func getMxRecord(domain string) ([]string, error) {
	var records []string
	mxRecord, err := net.LookupMX(domain)
	if err != nil {
		return nil, err
	}
	if len(mxRecord) == 0 {
		return nil, errors.New("mx record not found")
	}
	for _, record := range mxRecord {
		records = append(records, fmt.Sprintf("Host: %s\t\tPreference: %d\n", record.Host, record.Pref))
	}
	return records, nil
}

func getNsRecord(domain string) ([]string, error) {
	var records []string
	res, err := net.LookupNS(domain)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	if len(res) == 0 {
		return nil, errors.New("NS record not found")
	}
	for _, result := range res {
		records = append(records, fmt.Sprintf("%s\n", result.Host))
	}
	return records, nil
}

func getTxtRecord(domain string) ([]string, error) {
	txtRecord, err := net.LookupTXT(domain)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	for _, record := range txtRecord {
		if strings.HasPrefix(record, "v=spf1") || strings.HasPrefix(record, "v=DMARC1") {
			return txtRecord, nil
		}
	}
	return nil, errors.New("not found")
}

func printRecords(email Email, wg *sync.WaitGroup, printchannel chan<- string, sem chan struct{}) {
	defer wg.Done()
	sem <- struct{}{}
	if err := email.parse(); err != nil {
		return
	}
	output := getOutput(email)
	if mxRecord, err := getMxRecord(email.domain); err != nil {
		output += err.Error()
	} else {
		output += "-----------MX Records---------\n"
		for _, record := range mxRecord {
			output += record
		}
	}
	if nsRecord, err := getNsRecord(email.domain); err != nil {
		output += err.Error()
	} else {
		output += "-----------NS record----------\n"
		for _, record := range nsRecord {
			output += record
		}
	}

	if txtRecord, err := getTxtRecord(email.domain); err != nil {
		output += "SPF records " + err.Error()
	} else {
		output += "------------SPF records-----------\n"
		for _, record := range txtRecord {
			output += record + "\n"
		}
	}
	if txtRecord, err := getTxtRecord("_dmarc." + email.domain); err != nil {
		output += "DMARC records " + err.Error()
	} else {
		output += "-----------DMARC record-----------\n"
		for _, record := range txtRecord {
			output += record + "\n\n"
		}
	}

	printchannel <- output
	<-sem

}

var parseEmailAddress *bool = flag.Bool("p", false, "Just parse the email address and check if it is valid")

func main() {
	flag.Parse()
	if *parseEmailAddress {

		var maxGoroutines int = 5
		var wg = sync.WaitGroup{}
		var sem chan struct{} = make(chan struct{}, maxGoroutines)
		var printChannel = make(chan string)

		ParseEmailAddress(printChannel, sem, &wg)
		go func() {
			wg.Wait()
			close(printChannel)
		}()
		for output := range printChannel {
			fmt.Print(output)
		}
		return
	}
	var maxGoroutines int = 5
	var wg = sync.WaitGroup{}
	var sem chan struct{} = make(chan struct{}, maxGoroutines)
	var printChannel = make(chan string)

	for _, address := range os.Args[1:] {

		email := Email{address: address}
		wg.Add(1)
		go printRecords(email, &wg, printChannel, sem)
	}
	go func() {
		wg.Wait()
		close(printChannel)
	}()
	for output := range printChannel {
		fmt.Print(output)
	}
}
