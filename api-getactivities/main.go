package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
)

type AthleteSummary struct {
	Id             int64 `json:"id"`
	Resource_state int   `json:"resource_state"`
}

type Location [2]float64 // [longitude, latitude]

type ActivitySummary struct {
	Resource_state     int64          `json:"resource_state"` // 1 for “summary”, 2 for “detail”
	Athlete            AthleteSummary `json:"athlete"`
	Name               string         `json:"name"`
	Distance           float64        `json:"distance"`
	MovingTime         int            `json:"moving_time"`
	ElapsedTime        int            `json:"elapsed_time"`
	TotalElevationGain float64        `json:"total_elevation_gain"`
	Type               string         `json:"type"`
	WorkoutType        int            `json:"workout_type"`
	Id                 int64          `json:"id"`
	StartDate          string         `json:"start_date"`
	StartDateLocal     string         `json:"start_date_local"`
	TimeZone           string         `json:"timezone"`
	UtcOffset          int            `json:"utc_offset"`
	City               string         `json:"location_city"`
	State              string         `json:"location_state"`
	Country            string         `json:"location_country"`
	AchievementCount   int            `json:"achievement_count"`
	KudosCount         int            `json:"kudos_count"`
	CommentCount       int            `json:"comment_count"`
	AthleteCount       int            `json:"athlete_count"`
	PhotoCount         int            `json:"photo_count"`
	Map                struct {
		Id              string `json:"id"`
		SummaryPolyline string `json:"summary_polyline"`
		Resource_state  int    `json:"resource_state"`
	} `json:"map"`
	Trainer              bool     `json:"trainer"`
	Commute              bool     `json:"commute"`
	Manual               bool     `json:"manual"`
	Private              bool     `json:"private"`
	Visibility           string   `json:"visibility"`
	Flagged              bool     `json:"flagged"`
	GearId               string   `json:"gear_id"` // bike or pair of shoes
	StartLocation        Location `json:"start_latlng"`
	EndLocation          Location `json:"end_latlng"`
	AverageSpeed         float64  `json:"average_speed"`
	MaximunSpeed         float64  `json:"max_speed"`
	HasHeartrate         bool     `json:"has_heartrate"`
	HeartRateOptOut      bool     `json:"heartrate_opt_out"`
	DisplayHideHeartrate bool     `json:"display_hide_heartrate_option"`
	ElevHigh             float64  `json:"elev_high"`
	ElevLow              float64  `json:"elev_low"`
	UploadId             int64    `json:"upload_id"`
	UploadIdString       string   `json:"upload_id_str"`
	ExternalId           string   `json:"external_id"`
	FromAcceptedTag      bool     `json:"from_accepted_tag"`
	PrCount              int      `json:"pr_count"`
	TotalPhotoCount      int      `json:"total_photo_count"`
	HasKudoed            bool     `json:"has_kudoed"`
}

type FinalActivity struct {
	Distance       float64 `json:"distance"`
	MovingTime     int     `json:"moving_time"`
	StartDate      string  `json:"start_date"`
	StartDateLocal string  `json:"start_date_local"`
	StartDateUnix  int     `json:"start_date_unix"`
	TimeZone       string  `json:"timezone"`
	UtcOffset      int     `json:"utc_offset"`
	Miles          float64 `json:"miles"`
	Minutes        float64 `json:"minutes"`
	Pace           float64 `json:"pace"`
	DisplayPace    string  `json:"display_pace"`
}

type FinalActivities struct {
	Data []FinalActivity `json:"data"`
}

type AthleteCredentials = struct {
	Id             int64     `json:"id"`
	Username       string    `json:"username"`
	Resource_state int       `json:"resource_state"`
	Firstname      string    `json:"firstname"`
	Lastname       string    `json:"lastname"`
	Bio            string    `json:"bio"`
	City           string    `json:"city"`
	State          string    `json:"state"`
	Country        string    `json:"country"`
	Sex            string    `json:"sex"`
	Premium        bool      `json:"premium"`
	Summit         bool      `json:"summit"`
	Created_at     time.Time `json:"created_at"`
	Updated_at     time.Time `json:"updated_at"`
	Badge_type_id  int       `json:"badge_type_id"`
	Weight         float64   `json:"weight"`
	Profile_medium string    `json:"profile_medium"`
	Profile        string    `json:"profile"`
	Friend         bool      `json:"friend"`
	Follower       bool      `json:"follower"`
}

type Credentials = struct {
	Client_id     int                `json:"client_id"`
	Client_secret string             `json:"client_secret"`
	Token_type    string             `json:"token_type"`
	Expires_at    int64              `json:"expires_at"`
	Expires_in    int64              `json:"expires_in"`
	Refresh_token string             `json:"refresh_token"`
	Access_token  string             `json:"access_token"`
	Athlete       AthleteCredentials `json:"athlete"`
}

type Payload = struct {
	Client_id     int    `json:"client_id"`
	Client_secret string `json:"client_secret"`
	Refresh_token string `json:"refresh_token"`
	Grant_type    string `json:"grant_type"`
	F             string `json:"f"`
}

func getDataFromGCS(object string) []byte {

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		fmt.Println("storage broken")
	}

	rc, err := client.Bucket("personal-website-35-stava-api-prod").Object(object).NewReader(ctx)
	if err != nil {
		fmt.Println("bucket broken")
	}
	slurp, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		fmt.Println("ioutil broken")
	}
	return slurp
}

func getStravaData(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
	c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

	var creds Credentials

	creds_object := "credentials/strava_refresh_token.json"

	credsSlurp := getDataFromGCS(creds_object)

	json.Unmarshal(credsSlurp, &creds)

	clientID := creds.Client_id
	clientSecret := creds.Client_secret
	refreshToken := creds.Refresh_token

	client := &http.Client{}

	var payload Payload

	payload.Client_id = clientID
	payload.Client_secret = clientSecret
	payload.Refresh_token = refreshToken
	payload.Grant_type = "refresh_token"
	payload.F = "json"

	bytes_playload, err := json.Marshal(payload)

	var credsToUse Credentials

	refresh_req, err := http.NewRequest("POST", "https://www.strava.com/oauth/token", bytes.NewBuffer(bytes_playload))
	if err != nil {
		fmt.Println(err)
		return
	}

	refresh_req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(refresh_req)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer res.Body.Close()

	json.NewDecoder(res.Body).Decode(&credsToUse)

	access_token := credsToUse.Access_token

	activities_req, err := http.NewRequest("GET", "https://www.strava.com/api/v3/athlete/activities", nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	parm := activities_req.URL.Query()
	parm.Add("per_page", "30")
	parm.Add("page", "1")

	activities_req.URL.RawQuery = parm.Encode()

	activities_req.Header.Add("Authorization", "Bearer "+access_token)

	activities_res, err := client.Do(activities_req)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer activities_res.Body.Close()

	var athActs []ActivitySummary

	json.NewDecoder(activities_res.Body).Decode(&athActs)

	var finalActs FinalActivities

	for _, a := range athActs {
		var finalAct FinalActivity
		finalAct.Distance = a.Distance
		finalAct.MovingTime = a.MovingTime
		finalAct.StartDate = a.StartDate
		finalAct.StartDateLocal = a.StartDateLocal
		finalAct.TimeZone = a.TimeZone
		finalAct.UtcOffset = a.UtcOffset
		// convert zulu string time to unix time
		time_temp, err := time.Parse(time.RFC3339, a.StartDateLocal)
		if err != nil {
			fmt.Println(err.Error())
			panic(err)
		}
		finalAct.StartDateUnix = int(time_temp.Unix())
		miles := a.Distance * 0.000621371
		finalAct.Miles = miles
		minutes := float64(a.MovingTime) / 60
		finalAct.Minutes = minutes
		pace := minutes / miles
		hanging_decimal := pace - float64(int(pace))
		seconds := float64(math.Round(hanging_decimal * 60))
		finalAct.Pace = pace
		pace_down := float64(int(pace))
		if seconds < 10 {
			finalAct.DisplayPace = fmt.Sprintf("%.0f:0%.0f", pace_down, seconds)
		} else {
			finalAct.DisplayPace = fmt.Sprintf("%.0f:%.0f", pace_down, seconds)
		}

		finalActs.Data = append(finalActs.Data, finalAct)
	}

	c.IndentedJSON(http.StatusOK, finalActs)
}

const ContentTypeHTML = "text/html; charset=utf-8"

func getIndex(c *gin.Context) {
	c.Data(http.StatusOK, ContentTypeHTML, []byte("<html>The Strava API Application Works.</html>"))
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.GET("/strava", getStravaData)
	router.GET("/", getIndex)
	router.Run(":8080")
}
