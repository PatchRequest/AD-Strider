package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

var config AutoGenerated
var badJumps []TierConnection
var wg sync.WaitGroup

func main() {
	uri := flag.String("uri", "bolt://127.0.0.1:7687", "URL of neo4j server Default: bolt://127.0.0.1:7687")
	username := flag.String("username", "neo4j", "Username for neo4j server Default: neo4j")
	password := flag.String("password", "neo4j", "Password for neo4j server Default: neo4j")
	filepath := flag.String("filepath", "./tier0.txt", "Path to file containing list of elements to add tier flag to")
	flag.Parse()
	if !strings.HasPrefix(*uri, "bolt://") {
		*uri = "bolt://" + *uri
	}
	fmt.Println("Connecting to neo4j server at: ", *uri, " ...")
	// Connect to the database
	driver, err := neo4j.NewDriver(*uri, neo4j.BasicAuth(*username, *password, ""))
	if err != nil {
		panic(err)
	}
	defer driver.Close()
	fmt.Println("Reading Config file...")
	// Read the config file
	configData, err := os.ReadFile("./config.json")
	if err != nil {
		panic(err)
	}
	fmt.Println("Parsing Config file...")
	// Parse the Config file
	err = json.Unmarshal(configData, &config)
	if err != nil {
		panic(err)
	}
	fmt.Println("Cleaning current tier0 flag...")
	// clean current tier0
	err = cleanCurrentTier0(driver)
	if err != nil {
		panic(err)
	}
	fmt.Println("Reading tier0 file...")
	// Read Tier0 File
	dat, err := os.ReadFile(*filepath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Adding tier0 flag to elements...")
	// Add Tier0 Flag to elements
	for _, elementName := range strings.Split(string(dat), "\n") {
		go addTierFlagToElement(driver, elementName)

	}
	wg.Wait()
	fmt.Println("Analyzing all jumps...")
	// Check all jumps
	getAllJumps(driver)

	wg.Wait()
	fmt.Println("Writing bad jumps to file...")
	// Print out bad jumps
	t := strconv.FormatInt(int64(time.Now().Unix()), 10)
	f, err := os.OpenFile("badConnections"+t+".csv", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err = f.WriteString("Source, ConnectionType, Target, isACL\n"); err != nil {
		panic(err)
	}
	for _, jump := range badJumps {
		writeString := "" + jump.StartEntity.Props["name"].(string) + "," + jump.Relationship.Type + "," + jump.EndEntity.Props["name"].(string) + "," + strconv.FormatBool(jump.Relationship.Props["isacl"].(bool)) + "\n"
		if _, err = f.WriteString(writeString); err != nil {
			panic(err)
		}
	}

	println("Bad Jumps:", len(badJumps))

}

func cleanCurrentTier0(driver neo4j.Driver) error {
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	_, err := session.WriteTransaction(func(tx neo4j.Transaction) (interface{}, error) {

		result, err := tx.Run("MATCH (n) WHERE n.tier0=true REMOVE n.tier0 RETURN n", map[string]interface{}{})
		if err != nil {
			return nil, err
		}
		result.Consume()
		return nil, err
	})
	return err
}

func addTierFlagToElement(driver neo4j.Driver, elementName string) error {
	wg.Add(1)
	defer wg.Done()
	if len(elementName) < 3 {
		return nil
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})

	defer session.Close()

	_, err := session.WriteTransaction(func(tx neo4j.Transaction) (interface{}, error) {
		searchString := ""
		if strings.Contains(elementName, "@") {
			searchString = "MATCH (n) WHERE toLower(n.name) = toLower($elementName) SET n.tier0=true RETURN n"
		} else {
			searchString = "MATCH (n) WHERE toLower(n.name) CONTAINS toLower($elementName) SET n.tier0=true RETURN n"
		}
		result, err := tx.Run(searchString, map[string]interface{}{"elementName": elementName})
		if err != nil {
			return nil, err
		}
		// how many nodes were affected?
		affected, err := result.Consume()
		if err != nil {
			return nil, err
		}
		if affected.Counters().PropertiesSet() > 5 {
			fmt.Printf("High false positive marked rate for %s with %d nodes affected\n", elementName, affected.Counters().PropertiesSet())
		}

		return nil, err
	})

	return err
}

func getAllJumps(driver neo4j.Driver) error {

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})

	defer session.Close()

	_, err := session.WriteTransaction(func(tx neo4j.Transaction) (interface{}, error) {

		result, err := tx.Run("MATCH (m WHERE m.tier0) -[r]- (x WHERE NOT EXISTS(x.tier0) AND EXISTS(x.domain) ) return m,r,x", map[string]interface{}{})
		if err != nil {
			return nil, err
		}
		for result.Next() {
			record := result.Record()
			tier0_device := record.GetByIndex(0).(neo4j.Node)
			relationship := record.GetByIndex(1).(neo4j.Relationship)
			other_device := record.GetByIndex(2).(neo4j.Node)

			if relationship.EndId == other_device.Id {
				go anaylseConnection(TierConnection{StartEntity: tier0_device, Relationship: relationship, EndEntity: other_device, IntoT0: false})

			} else {
				go anaylseConnection(TierConnection{StartEntity: other_device, Relationship: relationship, EndEntity: tier0_device, IntoT0: true})
			}

		}

		return nil, err

	})

	return err

}

func anaylseConnection(connection TierConnection) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("panic occurred:", err)
		}
	}()
	wg.Add(1)
	defer wg.Done()
	if connection.IntoT0 {

		r := reflect.ValueOf(config.IntoT0)
		f := reflect.Indirect(r).FieldByName(connection.Relationship.Type).Bool()

		if f {
			badJumps = append(badJumps, connection)
		}

	} else {

		r := reflect.ValueOf(config.IntoT1)
		f := reflect.Indirect(r).FieldByName(connection.Relationship.Type).Bool()

		if f {
			badJumps = append(badJumps, connection)
		}

	}

}

type TierConnection struct {
	StartEntity  neo4j.Node
	Relationship neo4j.Relationship
	EndEntity    neo4j.Node
	IntoT0       bool
}

type AutoGenerated struct {
	IntoT0 struct {
		AdminTo                   bool `json:"AdminTo"`
		MemberOf                  bool `json:"MemberOf"`
		HasSession                bool `json:"HasSession"`
		ForceChangePassword       bool `json:"ForceChangePassword"`
		AddMembers                bool `json:"AddMembers"`
		AddSelf                   bool `json:"AddSelf"`
		CanRDP                    bool `json:"CanRDP"`
		CanPSRemote               bool `json:"CanPSRemote"`
		ExecuteDCOM               bool `json:"ExecuteDCOM"`
		SQLAdmin                  bool `json:"SQLAdmin"`
		AllowedToDelegate         bool `json:"AllowedToDelegate"`
		DCSync                    bool `json:"DCSync"`
		GetChanges                bool `json:"GetChanges"`
		GetChangesAll             bool `json:"GetChangesAll"`
		GenericAll                bool `json:"GenericAll"`
		WriteDacl                 bool `json:"WriteDacl"`
		GenericWrite              bool `json:"GenericWrite"`
		WriteOwner                bool `json:"WriteOwner"`
		WriteSPN                  bool `json:"WriteSPN"`
		Owns                      bool `json:"Owns"`
		AddKeyCredentialLink      bool `json:"AddKeyCredentialLink"`
		ReadLAPSPassword          bool `json:"ReadLAPSPassword"`
		ReadGMSAPassword          bool `json:"ReadGMSAPassword"`
		Contains                  bool `json:"Contains"`
		AllExtendedRights         bool `json:"AllExtendedRights"`
		GPLink                    bool `json:"GPLink"`
		AllowedToAct              bool `json:"AllowedToAct"`
		AddAllowedToAct           bool `json:"AddAllowedToAct"`
		TrustedBy                 bool `json:"TrustedBy"`
		SyncLAPSPassword          bool `json:"SyncLAPSPassword"`
		AZAddMembers              bool `json:"AZAddMembers"`
		AZAppAdmin                bool `json:"AZAppAdmin"`
		AZCloudAppAdmin           bool `json:"AZCloudAppAdmin"`
		AZContains                bool `json:"AZContains"`
		AZContributor             bool `json:"AZContributor"`
		AZGetCertificates         bool `json:"AZGetCertificates"`
		AZGetKeys                 bool `json:"AZGetKeys"`
		AZGetSecrets              bool `json:"AZGetSecrets"`
		AZGlobalAdmin             bool `json:"AZGlobalAdmin"`
		AZPrivilegedRoleAdmin     bool `json:"AZPrivilegedRoleAdmin"`
		AZResetPassword           bool `json:"AZResetPassword"`
		AZRunsAs                  bool `json:"AZRunsAs"`
		AZUserAccessAdministrator bool `json:"AZUserAccessAdministrator"`
	} `json:"IntoT0"`
	IntoT1 struct {
		AdminTo                   bool `json:"AdminTo"`
		MemberOf                  bool `json:"MemberOf"`
		HasSession                bool `json:"HasSession"`
		ForceChangePassword       bool `json:"ForceChangePassword"`
		AddMembers                bool `json:"AddMembers"`
		AddSelf                   bool `json:"AddSelf"`
		CanRDP                    bool `json:"CanRDP"`
		CanPSRemote               bool `json:"CanPSRemote"`
		ExecuteDCOM               bool `json:"ExecuteDCOM"`
		SQLAdmin                  bool `json:"SQLAdmin"`
		AllowedToDelegate         bool `json:"AllowedToDelegate"`
		DCSync                    bool `json:"DCSync"`
		GetChanges                bool `json:"GetChanges"`
		GetChangesAll             bool `json:"GetChangesAll"`
		GenericAll                bool `json:"GenericAll"`
		WriteDacl                 bool `json:"WriteDacl"`
		GenericWrite              bool `json:"GenericWrite"`
		WriteOwner                bool `json:"WriteOwner"`
		WriteSPN                  bool `json:"WriteSPN"`
		Owns                      bool `json:"Owns"`
		AddKeyCredentialLink      bool `json:"AddKeyCredentialLink"`
		ReadLAPSPassword          bool `json:"ReadLAPSPassword"`
		ReadGMSAPassword          bool `json:"ReadGMSAPassword"`
		Contains                  bool `json:"Contains"`
		AllExtendedRights         bool `json:"AllExtendedRights"`
		GPLink                    bool `json:"GPLink"`
		AllowedToAct              bool `json:"AllowedToAct"`
		AddAllowedToAct           bool `json:"AddAllowedToAct"`
		TrustedBy                 bool `json:"TrustedBy"`
		SyncLAPSPassword          bool `json:"SyncLAPSPassword"`
		AZAddMembers              bool `json:"AZAddMembers"`
		AZAppAdmin                bool `json:"AZAppAdmin"`
		AZCloudAppAdmin           bool `json:"AZCloudAppAdmin"`
		AZContains                bool `json:"AZContains"`
		AZContributor             bool `json:"AZContributor"`
		AZGetCertificates         bool `json:"AZGetCertificates"`
		AZGetKeys                 bool `json:"AZGetKeys"`
		AZGetSecrets              bool `json:"AZGetSecrets"`
		AZGlobalAdmin             bool `json:"AZGlobalAdmin"`
		AZPrivilegedRoleAdmin     bool `json:"AZPrivilegedRoleAdmin"`
		AZResetPassword           bool `json:"AZResetPassword"`
		AZRunsAs                  bool `json:"AZRunsAs"`
		AZUserAccessAdministrator bool `json:"AZUserAccessAdministrator"`
	} `json:"IntoT1"`
}
