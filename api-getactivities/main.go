package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
)

type AthleteDetailed struct {
	AthleteSummary
	Email                 string         `json:"email"`
	FollowerCount         int            `json:"follower_count"`
	FriendCount           int            `json:"friend_count"`
	MutualFriendCount     int            `json:"mutual_friend_count"`
	DatePreference        string         `json:"date_preference"`
	MeasurementPreference string         `json:"measurement_preference"`
	FTP                   int            `json:"ftp"`
	Weight                float64        `json:"weight"` // kilograms
	Clubs                 []*ClubSummary `json:"clubs"`
	Bikes                 []*GearSummary `json:"bikes"`
	Shoes                 []*GearSummary `json:"shoes"`
}

type AthleteSummary struct {
	AthleteMeta
	FirstName        string    `json:"firstname"`
	LastName         string    `json:"lastname"`
	ProfileMedium    string    `json:"profile_medium"` // URL to a 62x62 pixel profile picture
	Profile          string    `json:"profile"`        // URL to a 124x124 pixel profile picture
	City             string    `json:"city"`
	State            string    `json:"state"`
	Country          string    `json:"country"`
	Gender           Gender    `json:"sex"`
	Friend           string    `json:"friend"`   // ‘pending’, ‘accepted’, ‘blocked’ or ‘null’, the authenticated athlete’s following status of this athlete
	Follower         string    `json:"follower"` // this athlete’s following status of the authenticated athlete
	Premium          bool      `json:"premium"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	ApproveFollowers bool      `json:"approve_followers"` // if has enhanced privacy enabled
	BadgeTypeId      int       `json:"badge_type_id"`
}

type AthleteMeta struct {
	Id int64 `json:"id"`
}

type AthleteStats struct {
	BiggestRideDistance       float64       `json:"biggest_ride_distance"`
	BiggestClimbElevationGain float64       `json:"biggest_climb_elevation_gain"`
	RecentRideTotals          AthleteTotals `json:"recent_ride_totals"`
	RecentRunTotals           AthleteTotals `json:"recent_run_totals"`
	YTDRideTotals             AthleteTotals `json:"ytd_ride_totals"`
	YTDRunTotals              AthleteTotals `json:"ytd_run_totals"`
	AllRideTotals             AthleteTotals `json:"all_ride_totals"`
	AllRunTotals              AthleteTotals `json:"all_run_totals"`
}

type AthleteTotals struct {
	Count         int     `json:"count"`
	Distance      float64 `json:"distance"`
	MovingTime    int     `json:"moving_time"`
	ElapsedTime   int     `json:"elapsed_time"`
	ElevationGain float64 `json:"elevation_gain"`

	// only correct for recent totals, not ytd or all
	AchievementCount int `json:"achievement_count"`
}

type Gender string

var Genders = struct {
	Unspecified Gender
	Male        Gender
	Female      Gender
}{"", "M", "F"}

type ActivityDetailed struct {
	ActivitySummary
	Calories       float64                 `json:"calories"`
	Description    string                  `json:"description"`
	Gear           GearSummary             `json:"gear"`
	SegmentEfforts []*SegmentEffortSummary `json:"segment_efforts"`
	SplitsMetric   []*Split                `json:"splits_metric"`
	SplitsStandard []*Split                `json:"splits_standard"`
	BestEfforts    []*BestEffort           `json:"best_efforts"`
}

type ActivitySummary struct {
	Id                 int64          `json:"id"`
	ExternalId         string         `json:"external_id"`
	UploadId           int64          `json:"upload_id"`
	Athlete            AthleteSummary `json:"athlete"`
	Name               string         `json:"name"`
	Distance           float64        `json:"distance"`
	MovingTime         int            `json:"moving_time"`
	ElapsedTime        int            `json:"elapsed_time"`
	TotalElevationGain float64        `json:"total_elevation_gain"`
	Type               ActivityType   `json:"type"`

	StartDate      time.Time `json:"start_date"`
	StartDateLocal time.Time `json:"start_date_local"`

	TimeZone         string   `json:"time_zone"`
	StartLocation    Location `json:"start_latlng"`
	EndLocation      Location `json:"end_latlng"`
	City             string   `json:"location_city"`
	State            string   `json:"location_state"`
	Country          string   `json:"location_country"`
	AchievementCount int      `json:"achievement_count"`
	KudosCount       int      `json:"kudos_count"`
	CommentCount     int      `json:"comment_count"`
	AthleteCount     int      `json:"athlete_count"`
	PhotoCount       int      `json:"photo_count"`
	Map              struct {
		Id              string   `json:"id"`
		Polyline        Polyline `json:"polyline"`
		SummaryPolyline Polyline `json:"summary_polyline"`
	} `json:"map"`
	Trainer              bool    `json:"trainer"`
	Commute              bool    `json:"commute"`
	Manual               bool    `json:"manual"`
	Private              bool    `json:"private"`
	Flagged              bool    `json:"flagged"`
	GearId               string  `json:"gear_id"` // bike or pair of shoes
	AverageSpeed         float64 `json:"average_speed"`
	MaximunSpeed         float64 `json:"max_speed"`
	AverageCadence       float64 `json:"average_cadence"`
	AverageTemperature   float64 `json:"average_temp"`
	AveragePower         float64 `json:"average_watts"`
	WeightedAveragePower int     `json:"weighted_average_watts"`
	Kilojoules           float64 `json:"kilojoules"`
	DeviceWatts          bool    `json:"device_watts"`
	AverageHeartrate     float64 `json:"average_heartrate"`
	MaximumHeartrate     float64 `json:"max_heartrate"`
	Truncated            int     `json:"truncated"` // only present if activity is owned by authenticated athlete, returns 0 if not truncated by privacy zones
	HasKudoed            bool    `json:"has_kudoed"`
}

type BestEffort struct {
	EffortSummary
	PRRank int `json:"pr_rank"` // 1-3 personal record on segment at time of upload
}

type ActivityType string

var ActivityTypes = struct {
	Ride               ActivityType
	AlpineSki          ActivityType
	BackcountrySki     ActivityType
	Hike               ActivityType
	IceSkate           ActivityType
	InlineSkate        ActivityType
	NordicSki          ActivityType
	RollerSki          ActivityType
	Run                ActivityType
	Walk               ActivityType
	Workout            ActivityType
	Snowboard          ActivityType
	Snowshoe           ActivityType
	Kitesurf           ActivityType
	Windsurf           ActivityType
	Swim               ActivityType
	VirtualRide        ActivityType
	EBikeRide          ActivityType
	WaterSport         ActivityType
	Canoeing           ActivityType
	Kayaking           ActivityType
	Rowing             ActivityType
	StandUpPaddling    ActivityType
	Surfing            ActivityType
	Crossfit           ActivityType
	Elliptical         ActivityType
	RockClimbing       ActivityType
	StairStepper       ActivityType
	WeightTraining     ActivityType
	Yoga               ActivityType
	WinterSport        ActivityType
	CrossCountrySkiing ActivityType
}{"Ride", "AlpineSki", "BackcountrySki", "Hike", "IceSkate", "InlineSkate", "NordicSki", "RollerSki",
	"Run", "Walk", "Workout", "Snowboard", "Snowshoe", "Kitesurf", "Windsurf", "Swim", "VirtualRide", "EBikeRide",
	"WaterSport", "Canoeing", "Kayaking", "Rowing", "StandUpPaddling", "Surfing",
	"Crossfit", "Elliptical", "RockClimbing", "StairStepper", "WeightTraining", "Yoga", "WinterSport", "CrossCountrySkiing",
}

type Location [2]float64

var AthleteCredentials = struct {
	Id 	 int64  `json:"id"`
	Username string `json:"username"`
	Resource_state int `json:"resource_state"`
	Firstname string `json:"firstname"`
	Lastname string `json:"lastname"`
	Bio string `json:"bio"`
	City string `json:"city"`
	State string `json:"state"`
	Country string `json:"country"`
	Sex string `json:"sex"`
	Premium bool `json:"premium"`
	Summit bool `json:"summit"`
	Created_at time.Time `json:"created_at"`
	Updated_at time.Time `json:"updated_at"`
	Badge_type_id int `json:"badge_type_id"`
	Weight float64 `json:"weight"`
	Profile_medium string `json:"profile_medium"`
	Profile string `json:"profile"`
	Friend bool `json:"friend"`
	Follower bool `json:"follower"`
}

var Credentials = struct {
	Client_id int `json:"client_id"`
	Client_secret string `json:"client_secret"`
	Token_type   string `json:"token_type"`
	Expires_at   int64  `json:"expires_at"`
	Expires_in   int64  `json:"expires_in"`
	Refresh_token string `json:"refresh_token"`
	Access_token string `json:"access_token"`
	Athlete AthleteCredentials `json:"athlete"`
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

creds_object := "credentials/strava_refresh_token.json"

var creds Credentials

credsSlurp := getDataFromGCS(creds_object)

json.Unmarshal(credsSlurp, &creds)

func getStravaData(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
	c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

	clientID := creds.Client_id
	clientSecret := creds.Client_secret
	refreshToken := creds.Refresh_token

	client := &http.Client{}

	payload := fmt.Println("{'client_id': '", clientID, "', 'client_secret': '", clientSecret, "', 'refresh_token': '", refreshToken, "', 'grant_type': 'refresh_token', 'f': 'json'}")
	fmt.Println(payload)
	bytes_playload := []byte(payload)

	var credsToUse Credentials

	refresh_req, err := http.NewRequest("POST", "https://www.strava.com/oauth/token", bytes.NewBuffer(bytes_playload))
	if err != nil {
		fmt.Println(err)
		return
	}

	refresh_req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		panic(err)
	}

	defer res.Body.Close()

	json.Unmarshal(res, &credsToUse)

	var athlete AthleteDetailed

	athlete_object := "ATHLETE/production/athlete.json"

	athleteSlurp := getDataFromGCS(athlete_object)

	json.Unmarshal(athleteSlurp, &athlete)

	var activities []ActivityDetailed

	activities_object := "ACTIVITIES/production/activities.json"

	activitiesSlurp := getDataFromGCS(activities_object)

	json.Unmarshal(activitiesSlurp, &activities)

	c.JSON(http.StatusOK, gin.H{
		"athlete": athlete,
		"activities": activities,
	})
}

// func getStravaData(c *gin.Context) {
// 	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
// 	c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
// 	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
// 	c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")
// 	contextLeague := c.Query("league")
// 	contextService := c.Query("service")
// 	contextDraftgroup := c.Query("draftgroup")
// 	contextPosition := c.Query("position")
// 	contextDate := c.Query("date")
// 	contextTime := c.Query("time")
// 	contextTimeInt, err := strconv.ParseInt(contextTime, 10, 64)

// 	var draft_object string

// 	var oddsDat ODDSteam

// 	odds_object := "ODDS/production/ODDS_all.json"

// 	oddsSlurp := getDataFromGCS(odds_object)

// 	json.Unmarshal(oddsSlurp, &oddsDat)

// 	var draftables DKdraftables

// 	var players FDplayers

// 	if contextService == "draftkings" {
// 		draft_object = fmt.Sprint("DRAFTABLES/production/", contextDraftgroup, "_DK_", contextLeague, ".json")
// 		draftSlurp := getDataFromGCS(draft_object)
// 		json.Unmarshal(draftSlurp, &draftables)
// 	} else if contextService == "fanduel" {
// 		draft_object = fmt.Sprint("DRAFTABLES/production/", contextDraftgroup, "_FD_", contextLeague, ".json")
// 		draftSlurp := getDataFromGCS(draft_object)
// 		json.Unmarshal(draftSlurp, &players)

// 	}
// 	if err != nil {
// 		fmt.Println(err.Error())
// 	}

// 	var draft_week = checkifDateLessThan(contextDate)
// 	fmt.Println("Draft Week:", draft_week)

// 	var batters MLBbatters
// 	var pitchers MLBpitchers
// 	var nfl_players NFLplayersims

// 	if contextPosition == "pitchers" {
// 		simulation_object := fmt.Sprint(contextLeague, "/production/", contextDate, "_", contextLeague, "_", contextPosition, ".json")
// 		simSlurp := getDataFromGCS(simulation_object)
// 		json.Unmarshal(simSlurp, &pitchers)

// 	} else if contextPosition == "batters" {
// 		simulation_object := fmt.Sprint(contextLeague, "/production_with_regression/", contextDate, "_", contextLeague, "_", contextPosition, ".json")
// 		simSlurp := getDataFromGCS(simulation_object)
// 		json.Unmarshal(simSlurp, &batters)
// 	} else if contextPosition == "nfl_players" {
// 		simulation_object := fmt.Sprint(contextLeague, "/production_with_regression/2023_week_", draft_week, "_", contextLeague, "_players.json")
// 		simSlurp := getDataFromGCS(simulation_object)
// 		json.Unmarshal(simSlurp, &nfl_players)
// 		fmt.Println("NFL Players:", len(nfl_players))
// 	}

// 	var object_name string
// 	var slates SlatesData

// 	if contextLeague == "NFL" {
// 		if contextService == "draftkings" {
// 			object_name = "SLATES/production/DK_nfl.json"
// 			slateSlurp := getDataFromGCS(object_name)
// 			json.Unmarshal(slateSlurp, &slates)
// 		} else if contextService == "fanduel" {
// 			object_name = "SLATES/production/FD_allsports.json"
// 			slateSlurp := getDataFromGCS(object_name)
// 			json.Unmarshal(slateSlurp, &slates)
// 			fmt.Println("FD Slate:", len(slates))
// 		}
// 	}

// 	db := OpenConnection()

// 	var rows *sql.Rows

// 	if contextLeague == "MLB" {
// 		rows, err = db.Query("SELECT * FROM t_mlb_dfs_id_match;")
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 	} else if contextLeague == "NFL" {
// 		rows, err = db.Query("SELECT * FROM t_nfl_dfs_id_match;")
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 	}

// 	db.Close()

// 	Keys := map[string]ServiceKey{}

// 	for rows.Next() {
// 		var playerid int
// 		var playeridString string
// 		var key ServiceKey
// 		if contextLeague == "MLB" {
// 			if contextService == "draftkings" {
// 				rows.Scan(
// 					&key.PlayerID,
// 					&playerid,
// 					&key.Dkplayerid,
// 					&key.Fdid,
// 					&key.Yhid,
// 					&key.Sdid)
// 				key.Dkid = playerid
// 				Keys[fmt.Sprint(playerid)] = key
// 			} else if contextService == "fanduel" {
// 				rows.Scan(
// 					&key.PlayerID,
// 					&key.Dkid,
// 					&key.Dkplayerid,
// 					&playeridString,
// 					&key.Yhid,
// 					&key.Sdid)
// 				key.Fdid = playeridString
// 				Keys[playeridString] = key
// 			}
// 		} else if contextLeague == "NFL" {
// 			if contextService == "draftkings" {
// 				rows.Scan(
// 					&key.PlayerID,
// 					&playerid,
// 					&key.Dkplayerid,
// 					&key.Fdid)
// 				key.Dkid = playerid
// 				Keys[fmt.Sprint(playerid)] = key
// 			} else if contextService == "fanduel" {
// 				rows.Scan(
// 					&key.PlayerID,
// 					&key.Dkid,
// 					&key.Dkplayerid,
// 					&playeridString)
// 				key.Fdid = playeridString
// 				Keys[playeridString] = key
// 			}
// 		}
// 	}

// 	fmt.Println("Keys:", len(Keys))

// 	dbTeam := OpenConnection()

// 	rows_team, err := dbTeam.Query(
// 		"SELECT team, team_name, avg(runs_scored) as avg_points from t_mlb_games WHERE date > '2023-03-30' GROUP BY team, team_name ORDER by team ASC;",
// 	)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	dbTeam.Close()

// 	TeamKeys := map[string]TeamPoints{}

// 	for rows_team.Next() {
// 		var teamId int
// 		var teamKey TeamPoints
// 		rows_team.Scan(
// 			&teamId,
// 			&teamKey.Team_name,
// 			&teamKey.Avg_points)
// 		teamKey.TeamID = teamId
// 		TeamKeys[fmt.Sprint(teamId)] = teamKey
// 	}

// 	var FinalBatters Batters
// 	var FinalBatter Batter
// 	var FinalBatterKeys []string

// 	var FinalPitchers Pitchers
// 	var FinalPitcher Pitcher
// 	var FinalPitcherKeys []string

// 	var FinalNFLPlayers NflPlayers
// 	var FinalNFLPlayer NflPlayer
// 	var FinalNFLPlayerKeys []string

// 	var lookup int
// 	var nfl_lookup int
// 	var tmpTeamId string
// 	var player_injured bool
// 	var player_injury_status string

// 	var game_type string
// 	var fd_game_date string

// 	// loop through slates and pull out the slate that matches the draft group id and save it.
// 	if contextLeague == "NFL" {
// 		for _, s := range slates {
// 			if fmt.Sprint(s.Draft_group) == contextDraftgroup {
// 				game_type = s.Game_type
// 				fd_game_date = s.Date_string
// 				fmt.Println("Game Type:", game_type)
// 			}
// 		}
// 	} else {
// 		game_type = "Nothing"
// 		fd_game_date = "Nothing"
// 	}

// 	var defaultNFLPlayer NflPlayer

// 	defaultNFLPlayer.Full_name = "None"
// 	defaultNFLPlayer.Nfl_id = "0"
// 	defaultNFLPlayer.Player_current_weight = 0
// 	defaultNFLPlayer.Player_current_height = 0
// 	defaultNFLPlayer.Player_jersey_number = 0
// 	defaultNFLPlayer.Player_primary_position = "None"
// 	defaultNFLPlayer.Team_name = "None"
// 	defaultNFLPlayer.Positions = []string{"None"}
// 	defaultNFLPlayer.Inlineup = false
// 	defaultNFLPlayer.Status = "None"
// 	defaultNFLPlayer.Salary = 0
// 	defaultNFLPlayer.RosterSlotId = 0
// 	defaultNFLPlayer.Lineup_selected = 0
// 	defaultNFLPlayer.ProjPoints = 0
// 	defaultNFLPlayer.ProjPointsList = []int{0, 0, 0, 0, 0, 0, 0, 0}
// 	defaultNFLPlayer.Is_dome = 0
// 	defaultNFLPlayer.Is_home = 0
// 	defaultNFLPlayer.Temperature = 0
// 	defaultNFLPlayer.Humidity = 0
// 	defaultNFLPlayer.Wind_direction_norm = 0
// 	defaultNFLPlayer.Wind_speed = 0
// 	defaultNFLPlayer.Weather_summary = "None"
// 	defaultNFLPlayer.Game_abbr = "None"
// 	defaultNFLPlayer.Game_date_unix = 0
// 	defaultNFLPlayer.Game_gameDate = "None"
// 	defaultNFLPlayer.Game_name = "None"
// 	defaultNFLPlayer.Game_game_Pk = 0
// 	defaultNFLPlayer.Game_week = 0
// 	defaultNFLPlayer.Game_opponent = 0
// 	defaultNFLPlayer.Game_opponent_name = "None"
// 	defaultNFLPlayer.Game_player_oddsfactor = 0
// 	defaultNFLPlayer.Game_team_oddspoints = 0
// 	defaultNFLPlayer.Game_opponent_oddspoints = 0
// 	defaultNFLPlayer.Game_s_deviation = 0
// 	defaultNFLPlayer.Game_sim_cume_games = 0
// 	defaultNFLPlayer.Games_sim_cume_points = 0
// 	defaultNFLPlayer.Game_venue = 0
// 	defaultNFLPlayer.Game_venue_name = "None"
// 	defaultNFLPlayer.Stat_gen_fum = 0
// 	defaultNFLPlayer.Stat_gen_fum_lost = 0
// 	defaultNFLPlayer.Stat_gen_gp = 0
// 	defaultNFLPlayer.Stat_kick_fg_att = 0
// 	defaultNFLPlayer.Stat_kick_fg_long = 0
// 	defaultNFLPlayer.Stat_kick_fg_made = 0
// 	defaultNFLPlayer.Stat_kick_fg_pct = 0
// 	defaultNFLPlayer.Stat_kick_xp_att = 0
// 	defaultNFLPlayer.Stat_kick_xp_made = 0
// 	defaultNFLPlayer.Stat_kick_xp_pct = 0
// 	defaultNFLPlayer.Stat_qb_att = 0
// 	defaultNFLPlayer.Stat_qb_cmp = 0
// 	defaultNFLPlayer.Stat_qb_cmp_pct = 0
// 	defaultNFLPlayer.Stat_qb_int = 0
// 	defaultNFLPlayer.Stat_qb_rtg = 0
// 	defaultNFLPlayer.Stat_qb_tds = 0
// 	defaultNFLPlayer.Stat_qb_yds = 0
// 	defaultNFLPlayer.Stat_qb_yds_per_cmp = 0
// 	defaultNFLPlayer.Stat_qb_yds_per_game = 0
// 	defaultNFLPlayer.Stat_rec_rec = 0
// 	defaultNFLPlayer.Stat_rec_tds = 0
// 	defaultNFLPlayer.Stat_rec_tgt = 0
// 	defaultNFLPlayer.Stat_rec_yds = 0
// 	defaultNFLPlayer.Stat_rec_yds_per_game = 0
// 	defaultNFLPlayer.Stat_rec_yds_per_rec = 0
// 	defaultNFLPlayer.Stat_run_att = 0
// 	defaultNFLPlayer.Stat_run_tds = 0
// 	defaultNFLPlayer.Stat_run_yds = 0
// 	defaultNFLPlayer.Stat_run_yds_per_game = 0
// 	defaultNFLPlayer.Stat_run_yds_per_att = 0
// 	defaultNFLPlayer.Stat_def_int = 0
// 	defaultNFLPlayer.Stat_def_int_tds = 0
// 	defaultNFLPlayer.Stat_def_int_per_game = 0
// 	defaultNFLPlayer.Stat_def_kicks_blocked = 0
// 	defaultNFLPlayer.Stat_def_pass_defended = 0
// 	defaultNFLPlayer.Stat_def_sacks = 0
// 	defaultNFLPlayer.Stat_def_sacks_per_game = 0
// 	defaultNFLPlayer.Stat_def_safeties = 0
// 	defaultNFLPlayer.Stat_def_stuffs = 0
// 	defaultNFLPlayer.Stat_def_tackles = 0
// 	defaultNFLPlayer.Stat_def_tackles_for_loss = 0
// 	defaultNFLPlayer.Stat_def_tds = 0
// 	defaultNFLPlayer.Stat_def_turnover_diff = 0
// 	defaultNFLPlayer.Stat_kick_ko_returns_td = 0
// 	defaultNFLPlayer.Stat_punt_returns_tds = 0
// 	defaultNFLPlayer.Stat_def_pa_per_game_16g = 0
// 	defaultNFLPlayer.Stat_def_pa_per_game_4g = 0
// 	defaultNFLPlayer.Stat_def_pa_per_game_1g = 0
// 	defaultNFLPlayer.Def_cupcake = false
// 	defaultNFLPlayer.Def_trend_up_bool = false
// 	defaultNFLPlayer.Def_tough_bool = false

// 	if (contextPosition == "nfl_players") && (contextService == "fanduel") && (strings.Contains(game_type, "FullRoster")) {
// 		for _, s := range players.Players {
// 			FDidLookup := strings.Split(s.Id, "-")[1]
// 			nfl_lookup = Keys[fmt.Sprint(FDidLookup)].PlayerID
// 			FinalNFLPlayer = defaultNFLPlayer
// 			tmpTeamId = "None"
// 			FinalNFLPlayer.Full_name = s.First_name + " " + s.Last_name
// 			FinalNFLPlayer.Nfl_id = fmt.Sprint(nfl_lookup)
// 			FinalNFLPlayer.Player_current_weight = nfl_players[fmt.Sprint(nfl_lookup)].Player_current_weight
// 			FinalNFLPlayer.Player_current_height = nfl_players[fmt.Sprint(nfl_lookup)].Player_current_height
// 			FinalNFLPlayer.Player_jersey_number = nfl_players[fmt.Sprint(nfl_lookup)].Player_jersey
// 			FinalNFLPlayer.Player_primary_position = nfl_players[fmt.Sprint(nfl_lookup)].Player_position
// 			FinalNFLPlayer.Team_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_team_name
// 			FinalNFLPlayer.Positions = s.Positions
// 			for i, p := range FinalNFLPlayer.Positions {
// 				if p == "D" {
// 					p = "def"
// 				}
// 				FinalNFLPlayer.Positions[i] = strings.ToLower(p)
// 			}
// 			// if Positions contains RB, WR, or TE, add FLEX
// 			if implContains(FinalNFLPlayer.Positions, "rb") || implContains(FinalNFLPlayer.Positions, "wr") || implContains(FinalNFLPlayer.Positions, "te") || implContains(FinalNFLPlayer.Positions, "pk") || implContains(FinalNFLPlayer.Positions, "k") {
// 				FinalNFLPlayer.Positions = append(FinalNFLPlayer.Positions, "flex")
// 			}
// 			FinalNFLPlayer.Inlineup = s.Draftable
// 			player_injured = s.Injured
// 			if player_injured {
// 				player_injury_status = s.Injury_status
// 			} else {
// 				player_injury_status = "None"
// 			}
// 			if s.Position == "D" || s.Position == "DEF" {
// 				FinalNFLPlayer.Status = "None"
// 			} else {
// 				FinalNFLPlayer.Status = player_injury_status
// 			}
// 			FinalNFLPlayer.Salary = s.Salary
// 			FinalNFLPlayer.RosterSlotId = 0
// 			FinalNFLPlayer.Lineup_selected = 0
// 			FinalNFLPlayer.ProjPoints = nfl_players[fmt.Sprint(nfl_lookup)].Fd_classic_points_mean

// 			t_lessthan0 := 0
// 			t_one_nine := 0
// 			t_ten_nineteen := 0
// 			t_twt_twtnine := 0
// 			t_thr_thrnine := 0
// 			t_for_fornine := 0
// 			t_fft_fftnine := 0
// 			t_sixty_plus := 0

// 			for _, value := range nfl_players[fmt.Sprint(nfl_lookup)].Fd_classic_points_list {
// 				if value <= 0 {
// 					t_lessthan0 += 1
// 				} else if value >= 0 && value < 10 {
// 					t_one_nine += 1
// 				} else if value >= 10 && value < 20 {
// 					t_ten_nineteen += 1
// 				} else if value >= 20 && value < 30 {
// 					t_twt_twtnine += 1
// 				} else if value >= 30 && value < 40 {
// 					t_thr_thrnine += 1
// 				} else if value >= 40 && value < 50 {
// 					t_for_fornine += 1
// 				} else if value >= 50 && value < 60 {
// 					t_fft_fftnine += 1
// 				} else if value >= 60 {
// 					t_sixty_plus += 1
// 				}
// 			}
// 			var tempPointsList []int
// 			tempPointsList = append(tempPointsList, t_lessthan0)
// 			tempPointsList = append(tempPointsList, t_one_nine)
// 			tempPointsList = append(tempPointsList, t_ten_nineteen)
// 			tempPointsList = append(tempPointsList, t_twt_twtnine)
// 			tempPointsList = append(tempPointsList, t_thr_thrnine)
// 			tempPointsList = append(tempPointsList, t_for_fornine)
// 			tempPointsList = append(tempPointsList, t_fft_fftnine)
// 			tempPointsList = append(tempPointsList, t_sixty_plus)
// 			FinalNFLPlayer.ProjPointsList = tempPointsList
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Game_is_dome {
// 				FinalNFLPlayer.Is_dome = 1
// 			} else {
// 				FinalNFLPlayer.Is_dome = 0
// 			}
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Game_is_home {
// 				FinalNFLPlayer.Is_home = 1
// 			} else {
// 				FinalNFLPlayer.Is_home = 0
// 			}
// 			FinalNFLPlayer.Temperature = nfl_players[fmt.Sprint(nfl_lookup)].Temperature
// 			FinalNFLPlayer.Humidity = nfl_players[fmt.Sprint(nfl_lookup)].Humidity
// 			FinalNFLPlayer.Wind_direction_norm = nfl_players[fmt.Sprint(nfl_lookup)].Wind_direction_norm
// 			FinalNFLPlayer.Wind_speed = nfl_players[fmt.Sprint(nfl_lookup)].Wind_speed
// 			FinalNFLPlayer.Weather_summary = nfl_players[fmt.Sprint(nfl_lookup)].Weather_summary
// 			FinalNFLPlayer.Game_abbr = nfl_players[fmt.Sprint(nfl_lookup)].Game_abbr
// 			FinalNFLPlayer.Game_date_unix = nfl_players[fmt.Sprint(nfl_lookup)].Game_date_unix
// 			FinalNFLPlayer.Game_gameDate = fd_game_date
// 			FinalNFLPlayer.Game_game_Pk = 1
// 			FinalNFLPlayer.Game_week = nfl_players[fmt.Sprint(nfl_lookup)].Game_week
// 			FinalNFLPlayer.Game_opponent = nfl_players[fmt.Sprint(nfl_lookup)].Game_opp_id
// 			FinalNFLPlayer.Game_opponent_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_opp_name
// 			FinalNFLPlayer.Game_player_oddsfactor = 1.0
// 			FinalNFLPlayer.Game_team_oddspoints = nfl_players[fmt.Sprint(nfl_lookup)].Game_points_for
// 			FinalNFLPlayer.Game_opponent_oddspoints = nfl_players[fmt.Sprint(nfl_lookup)].Game_points_against
// 			FinalNFLPlayer.Game_s_deviation = nfl_players[fmt.Sprint(nfl_lookup)].Fd_classic_points_std
// 			FinalNFLPlayer.Game_sim_cume_games = nfl_players[fmt.Sprint(nfl_lookup)].Cume_sim_games
// 			FinalNFLPlayer.Games_sim_cume_points = nfl_players[fmt.Sprint(nfl_lookup)].Fd_classic_cume_points
// 			FinalNFLPlayer.Game_venue = nfl_players[fmt.Sprint(nfl_lookup)].Game_venue_id
// 			FinalNFLPlayer.Game_venue_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_venue_name

// 			// add in default statistics
// 			FinalNFLPlayer.Stat_gen_fum = 0
// 			FinalNFLPlayer.Stat_gen_fum_lost = 0
// 			FinalNFLPlayer.Stat_gen_gp = 0
// 			FinalNFLPlayer.Stat_kick_fg_att = 0
// 			FinalNFLPlayer.Stat_kick_fg_long = 0
// 			FinalNFLPlayer.Stat_kick_fg_made = 0
// 			FinalNFLPlayer.Stat_kick_fg_pct = 0
// 			FinalNFLPlayer.Stat_kick_xp_att = 0
// 			FinalNFLPlayer.Stat_kick_xp_made = 0
// 			FinalNFLPlayer.Stat_kick_xp_pct = 0
// 			FinalNFLPlayer.Stat_qb_att = 0
// 			FinalNFLPlayer.Stat_qb_cmp = 0
// 			FinalNFLPlayer.Stat_qb_cmp_pct = 0
// 			FinalNFLPlayer.Stat_qb_int = 0
// 			FinalNFLPlayer.Stat_qb_rtg = 0
// 			FinalNFLPlayer.Stat_qb_tds = 0
// 			FinalNFLPlayer.Stat_qb_yds = 0
// 			FinalNFLPlayer.Stat_qb_yds_per_cmp = 0
// 			FinalNFLPlayer.Stat_qb_yds_per_game = 0
// 			FinalNFLPlayer.Stat_rec_rec = 0
// 			FinalNFLPlayer.Stat_rec_tds = 0
// 			FinalNFLPlayer.Stat_rec_tgt = 0
// 			FinalNFLPlayer.Stat_rec_yds = 0
// 			FinalNFLPlayer.Stat_rec_yds_per_game = 0
// 			FinalNFLPlayer.Stat_rec_yds_per_rec = 0
// 			FinalNFLPlayer.Stat_run_att = 0
// 			FinalNFLPlayer.Stat_run_tds = 0
// 			FinalNFLPlayer.Stat_run_yds = 0
// 			FinalNFLPlayer.Stat_run_yds_per_att = 0
// 			FinalNFLPlayer.Stat_run_yds_per_game = 0
// 			FinalNFLPlayer.Stat_def_int = 0
// 			FinalNFLPlayer.Stat_def_int_tds = 0
// 			FinalNFLPlayer.Stat_def_int_per_game = 0
// 			FinalNFLPlayer.Stat_def_kicks_blocked = 0
// 			FinalNFLPlayer.Stat_def_pass_defended = 0
// 			FinalNFLPlayer.Stat_def_sacks = 0
// 			FinalNFLPlayer.Stat_def_sacks_per_game = 0
// 			FinalNFLPlayer.Stat_def_tackles = 0
// 			FinalNFLPlayer.Stat_def_tackles_for_loss = 0
// 			FinalNFLPlayer.Stat_def_tds = 0
// 			FinalNFLPlayer.Stat_def_turnover_diff = 0
// 			FinalNFLPlayer.Stat_kick_ko_returns_td = 0
// 			FinalNFLPlayer.Stat_punt_returns_tds = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_16g = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_4g = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_1g = 0

// 			// add in statistics if they exist
// 			FinalNFLPlayer.Stat_gen_fum = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_fum
// 			FinalNFLPlayer.Stat_gen_fum_lost = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_fum_lost
// 			FinalNFLPlayer.Stat_gen_gp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 			FinalNFLPlayer.Stat_kick_fg_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_att
// 			FinalNFLPlayer.Stat_kick_fg_long = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_long
// 			FinalNFLPlayer.Stat_kick_fg_made = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_made
// 			FinalNFLPlayer.Stat_kick_fg_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_pct
// 			FinalNFLPlayer.Stat_kick_xp_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_att
// 			FinalNFLPlayer.Stat_kick_xp_made = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_made
// 			FinalNFLPlayer.Stat_kick_xp_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_pct
// 			FinalNFLPlayer.Stat_qb_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_att
// 			FinalNFLPlayer.Stat_qb_cmp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_cmp
// 			FinalNFLPlayer.Stat_qb_cmp_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_cmp_pct
// 			FinalNFLPlayer.Stat_qb_int = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_int
// 			FinalNFLPlayer.Stat_qb_rtg = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_rtg
// 			FinalNFLPlayer.Stat_qb_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_tds
// 			FinalNFLPlayer.Stat_qb_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds
// 			FinalNFLPlayer.Stat_qb_yds_per_cmp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds_per_cmp
// 			FinalNFLPlayer.Stat_rec_rec = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_rec
// 			FinalNFLPlayer.Stat_rec_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_tds
// 			FinalNFLPlayer.Stat_rec_tgt = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_tgt
// 			FinalNFLPlayer.Stat_rec_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds
// 			FinalNFLPlayer.Stat_rec_yds_per_rec = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds_per_rec
// 			FinalNFLPlayer.Stat_run_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_att
// 			FinalNFLPlayer.Stat_run_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_tds
// 			FinalNFLPlayer.Stat_run_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds
// 			FinalNFLPlayer.Stat_run_yds_per_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds_per_att
// 			FinalNFLPlayer.Stat_def_int = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int
// 			FinalNFLPlayer.Stat_def_int_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int_tds
// 			FinalNFLPlayer.Stat_def_kicks_blocked = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_kicks_blocked
// 			FinalNFLPlayer.Stat_def_pass_defended = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_pass_defended
// 			FinalNFLPlayer.Stat_def_sacks = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_sacks
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp == 0 {
// 				FinalNFLPlayer.Stat_def_sacks_per_game = 0
// 				FinalNFLPlayer.Stat_qb_yds_per_game = 0
// 				FinalNFLPlayer.Stat_rec_yds_per_game = 0
// 				FinalNFLPlayer.Stat_run_yds_per_game = 0
// 				FinalNFLPlayer.Stat_def_int_per_game = 0
// 			} else {
// 				FinalNFLPlayer.Stat_def_sacks_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_sacks / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_qb_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_rec_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_run_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_def_int_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 			}
// 			FinalNFLPlayer.Stat_def_tackles = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tackles
// 			FinalNFLPlayer.Stat_def_tackles_for_loss = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tackles_for_loss
// 			FinalNFLPlayer.Stat_def_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tds
// 			FinalNFLPlayer.Stat_def_turnover_diff = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_turnover_diff
// 			FinalNFLPlayer.Stat_kick_ko_returns_td = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_ko_returns_td
// 			FinalNFLPlayer.Stat_punt_returns_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Punt_returns_tds
// 			FinalNFLPlayer.Stat_def_pa_per_game_16g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_16g
// 			FinalNFLPlayer.Stat_def_pa_per_game_4g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_4g
// 			FinalNFLPlayer.Stat_def_pa_per_game_1g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_1g

// 			// add in draftable unique id and final values
// 			FinalNFLPlayer.Draftable_uid = fmt.Sprint(s.Id, "_", nfl_lookup, "_", s.Position, "_", "_", FinalNFLPlayer.Nfl_id, "_", FinalNFLPlayer.Salary)
// 			FinalNFLPlayer.Def_cupcake = nfl_players[fmt.Sprint(nfl_lookup)].Def_cupcake
// 			FinalNFLPlayer.Def_trend_up_bool = nfl_players[fmt.Sprint(nfl_lookup)].Def_trend_up_bool
// 			FinalNFLPlayer.Def_tough_bool = nfl_players[fmt.Sprint(nfl_lookup)].Def_tough_bool

// 			if implContains(FinalNFLPlayerKeys, FinalNFLPlayer.Draftable_uid) {
// 			} else if !FinalNFLPlayer.Inlineup {
// 			} else {
// 				FinalNFLPlayerKeys = append(FinalNFLPlayerKeys, FinalNFLPlayer.Draftable_uid)
// 				FinalNFLPlayers.Data = append(FinalNFLPlayers.Data, FinalNFLPlayer)
// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalNFLPlayers)
// 	}

// 	if (contextPosition == "nfl_players") && (contextService == "draftkings") && (strings.Contains(game_type, "Classic")) {
// 		for _, s := range draftables.Draftables {
// 			FinalNFLPlayer = defaultNFLPlayer
// 			nfl_lookup = 0
// 			tmpTeamId = "None"
// 			nfl_lookup = Keys[fmt.Sprint(s.PlayerDkId)].PlayerID
// 			tmpTeamId = fmt.Sprint(nfl_players[fmt.Sprint(nfl_lookup)].Game_team_id)
// 			FinalNFLPlayer.Full_name = s.DisplayName
// 			FinalNFLPlayer.Nfl_id = fmt.Sprint(nfl_lookup)
// 			FinalNFLPlayer.Player_current_weight = nfl_players[fmt.Sprint(nfl_lookup)].Player_current_weight
// 			FinalNFLPlayer.Player_current_height = nfl_players[fmt.Sprint(nfl_lookup)].Player_current_height
// 			FinalNFLPlayer.Player_jersey_number = nfl_players[fmt.Sprint(nfl_lookup)].Player_jersey
// 			FinalNFLPlayer.Player_primary_position = nfl_players[fmt.Sprint(nfl_lookup)].Player_position
// 			FinalNFLPlayer.Team_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_team_name
// 			FinalNFLPlayer.Positions = strings.Split(s.Position, `/`)
// 			for i, p := range FinalNFLPlayer.Positions {
// 				FinalNFLPlayer.Positions[i] = strings.ToLower(p)
// 			}
// 			if implContains(FinalNFLPlayer.Positions, "rb") || implContains(FinalNFLPlayer.Positions, "wr") || implContains(FinalNFLPlayer.Positions, "te") || implContains(FinalNFLPlayer.Positions, "pk") || implContains(FinalNFLPlayer.Positions, "k") {
// 				FinalNFLPlayer.Positions = append(FinalNFLPlayer.Positions, "flex")
// 			}
// 			FinalNFLPlayer.Inlineup = true
// 			for _, value := range s.PlayerGameAttributes {
// 				if value.Id == 100 {
// 					if value.Value == "false" {
// 						FinalNFLPlayer.Inlineup = false
// 					}
// 				}
// 			}
// 			if s.Position == "D" || s.Position == "DEF" {
// 				FinalNFLPlayer.Status = "None"
// 			} else {
// 				FinalNFLPlayer.Status = s.Status
// 			}
// 			FinalNFLPlayer.Salary = s.Salary
// 			FinalNFLPlayer.RosterSlotId = s.RosterSlotId
// 			FinalNFLPlayer.Lineup_selected = 0
// 			FinalNFLPlayer.ProjPoints = nfl_players[fmt.Sprint(nfl_lookup)].Dk_classic_points_mean

// 			t_lessthan0 := 0
// 			t_one_nine := 0
// 			t_ten_nineteen := 0
// 			t_twt_twtnine := 0
// 			t_thr_thrnine := 0
// 			t_for_fornine := 0
// 			t_fft_fftnine := 0
// 			t_sixty_plus := 0

// 			for _, value := range nfl_players[fmt.Sprint(nfl_lookup)].Dk_classic_points_list {
// 				if value <= 0 {
// 					t_lessthan0 += 1
// 				} else if value >= 0 && value < 10 {
// 					t_one_nine += 1
// 				} else if value >= 10 && value < 20 {
// 					t_ten_nineteen += 1
// 				} else if value >= 20 && value < 30 {
// 					t_twt_twtnine += 1
// 				} else if value >= 30 && value < 40 {
// 					t_thr_thrnine += 1
// 				} else if value >= 40 && value < 50 {
// 					t_for_fornine += 1
// 				} else if value >= 50 && value < 60 {
// 					t_fft_fftnine += 1
// 				} else if value >= 60 {
// 					t_sixty_plus += 1
// 				}
// 			}
// 			var tempPointsList []int
// 			tempPointsList = append(tempPointsList, t_lessthan0)
// 			tempPointsList = append(tempPointsList, t_one_nine)
// 			tempPointsList = append(tempPointsList, t_ten_nineteen)
// 			tempPointsList = append(tempPointsList, t_twt_twtnine)
// 			tempPointsList = append(tempPointsList, t_thr_thrnine)
// 			tempPointsList = append(tempPointsList, t_for_fornine)
// 			tempPointsList = append(tempPointsList, t_fft_fftnine)
// 			tempPointsList = append(tempPointsList, t_sixty_plus)
// 			FinalNFLPlayer.ProjPointsList = tempPointsList
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Game_is_dome {
// 				FinalNFLPlayer.Is_dome = 1
// 			} else {
// 				FinalNFLPlayer.Is_dome = 0
// 			}
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Game_is_home {
// 				FinalNFLPlayer.Is_home = 1
// 			} else {
// 				FinalNFLPlayer.Is_home = 0
// 			}
// 			FinalNFLPlayer.Temperature = nfl_players[fmt.Sprint(nfl_lookup)].Temperature
// 			FinalNFLPlayer.Humidity = nfl_players[fmt.Sprint(nfl_lookup)].Humidity
// 			FinalNFLPlayer.Wind_direction_norm = nfl_players[fmt.Sprint(nfl_lookup)].Wind_direction_norm
// 			FinalNFLPlayer.Wind_speed = nfl_players[fmt.Sprint(nfl_lookup)].Wind_speed
// 			FinalNFLPlayer.Weather_summary = nfl_players[fmt.Sprint(nfl_lookup)].Weather_summary
// 			FinalNFLPlayer.Game_abbr = nfl_players[fmt.Sprint(nfl_lookup)].Game_abbr
// 			FinalNFLPlayer.Game_date_unix = nfl_players[fmt.Sprint(nfl_lookup)].Game_date_unix
// 			FinalNFLPlayer.Game_gameDate = s.Competition.StartTime
// 			FinalNFLPlayer.Game_game_Pk = 1
// 			FinalNFLPlayer.Game_week = nfl_players[fmt.Sprint(nfl_lookup)].Game_week
// 			FinalNFLPlayer.Game_opponent = nfl_players[fmt.Sprint(nfl_lookup)].Game_opp_id
// 			FinalNFLPlayer.Game_opponent_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_opp_name
// 			FinalNFLPlayer.Game_player_oddsfactor = 1.0
// 			FinalNFLPlayer.Game_team_oddspoints = nfl_players[fmt.Sprint(nfl_lookup)].Game_points_for
// 			FinalNFLPlayer.Game_opponent_oddspoints = nfl_players[fmt.Sprint(nfl_lookup)].Game_points_against
// 			FinalNFLPlayer.Game_s_deviation = nfl_players[fmt.Sprint(nfl_lookup)].Dk_classic_points_std
// 			FinalNFLPlayer.Game_sim_cume_games = nfl_players[fmt.Sprint(nfl_lookup)].Cume_sim_games
// 			FinalNFLPlayer.Games_sim_cume_points = nfl_players[fmt.Sprint(nfl_lookup)].Dk_classic_cume_points
// 			FinalNFLPlayer.Game_venue = nfl_players[fmt.Sprint(nfl_lookup)].Game_venue_id
// 			FinalNFLPlayer.Game_venue_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_venue_name

// 			// add in default statistics
// 			FinalNFLPlayer.Stat_gen_fum = 0
// 			FinalNFLPlayer.Stat_gen_fum_lost = 0
// 			FinalNFLPlayer.Stat_gen_gp = 0
// 			FinalNFLPlayer.Stat_kick_fg_att = 0
// 			FinalNFLPlayer.Stat_kick_fg_long = 0
// 			FinalNFLPlayer.Stat_kick_fg_made = 0
// 			FinalNFLPlayer.Stat_kick_fg_pct = 0
// 			FinalNFLPlayer.Stat_kick_xp_att = 0
// 			FinalNFLPlayer.Stat_kick_xp_made = 0
// 			FinalNFLPlayer.Stat_kick_xp_pct = 0
// 			FinalNFLPlayer.Stat_qb_att = 0
// 			FinalNFLPlayer.Stat_qb_cmp = 0
// 			FinalNFLPlayer.Stat_qb_cmp_pct = 0
// 			FinalNFLPlayer.Stat_qb_int = 0
// 			FinalNFLPlayer.Stat_qb_rtg = 0
// 			FinalNFLPlayer.Stat_qb_tds = 0
// 			FinalNFLPlayer.Stat_qb_yds = 0
// 			FinalNFLPlayer.Stat_qb_yds_per_cmp = 0
// 			FinalNFLPlayer.Stat_qb_yds_per_game = 0
// 			FinalNFLPlayer.Stat_rec_rec = 0
// 			FinalNFLPlayer.Stat_rec_tds = 0
// 			FinalNFLPlayer.Stat_rec_tgt = 0
// 			FinalNFLPlayer.Stat_rec_yds = 0
// 			FinalNFLPlayer.Stat_rec_yds_per_game = 0
// 			FinalNFLPlayer.Stat_rec_yds_per_rec = 0
// 			FinalNFLPlayer.Stat_run_att = 0
// 			FinalNFLPlayer.Stat_run_tds = 0
// 			FinalNFLPlayer.Stat_run_yds = 0
// 			FinalNFLPlayer.Stat_run_yds_per_att = 0
// 			FinalNFLPlayer.Stat_run_yds_per_game = 0
// 			FinalNFLPlayer.Stat_def_int = 0
// 			FinalNFLPlayer.Stat_def_int_tds = 0
// 			FinalNFLPlayer.Stat_def_int_per_game = 0
// 			FinalNFLPlayer.Stat_def_kicks_blocked = 0
// 			FinalNFLPlayer.Stat_def_pass_defended = 0
// 			FinalNFLPlayer.Stat_def_sacks = 0
// 			FinalNFLPlayer.Stat_def_sacks_per_game = 0
// 			FinalNFLPlayer.Stat_def_tackles = 0
// 			FinalNFLPlayer.Stat_def_tackles_for_loss = 0
// 			FinalNFLPlayer.Stat_def_tds = 0
// 			FinalNFLPlayer.Stat_def_turnover_diff = 0
// 			FinalNFLPlayer.Stat_kick_ko_returns_td = 0
// 			FinalNFLPlayer.Stat_punt_returns_tds = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_16g = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_4g = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_1g = 0

// 			// add in statistics if they exist
// 			FinalNFLPlayer.Stat_gen_fum = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_fum
// 			FinalNFLPlayer.Stat_gen_fum_lost = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_fum_lost
// 			FinalNFLPlayer.Stat_gen_gp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 			FinalNFLPlayer.Stat_kick_fg_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_att
// 			FinalNFLPlayer.Stat_kick_fg_long = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_long
// 			FinalNFLPlayer.Stat_kick_fg_made = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_made
// 			FinalNFLPlayer.Stat_kick_fg_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_pct
// 			FinalNFLPlayer.Stat_kick_xp_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_att
// 			FinalNFLPlayer.Stat_kick_xp_made = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_made
// 			FinalNFLPlayer.Stat_kick_xp_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_pct
// 			FinalNFLPlayer.Stat_qb_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_att
// 			FinalNFLPlayer.Stat_qb_cmp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_cmp
// 			FinalNFLPlayer.Stat_qb_cmp_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_cmp_pct
// 			FinalNFLPlayer.Stat_qb_int = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_int
// 			FinalNFLPlayer.Stat_qb_rtg = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_rtg
// 			FinalNFLPlayer.Stat_qb_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_tds
// 			FinalNFLPlayer.Stat_qb_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds
// 			FinalNFLPlayer.Stat_qb_yds_per_cmp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds_per_cmp
// 			FinalNFLPlayer.Stat_rec_rec = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_rec
// 			FinalNFLPlayer.Stat_rec_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_tds
// 			FinalNFLPlayer.Stat_rec_tgt = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_tgt
// 			FinalNFLPlayer.Stat_rec_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds
// 			FinalNFLPlayer.Stat_rec_yds_per_rec = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds_per_rec
// 			FinalNFLPlayer.Stat_run_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_att
// 			FinalNFLPlayer.Stat_run_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_tds
// 			FinalNFLPlayer.Stat_run_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds
// 			FinalNFLPlayer.Stat_run_yds_per_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds_per_att
// 			FinalNFLPlayer.Stat_def_int = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int
// 			FinalNFLPlayer.Stat_def_int_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int_tds
// 			FinalNFLPlayer.Stat_def_kicks_blocked = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_kicks_blocked
// 			FinalNFLPlayer.Stat_def_pass_defended = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_pass_defended
// 			FinalNFLPlayer.Stat_def_sacks = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_sacks
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp == 0 {
// 				FinalNFLPlayer.Stat_def_sacks_per_game = 0
// 				FinalNFLPlayer.Stat_qb_yds_per_game = 0
// 				FinalNFLPlayer.Stat_rec_yds_per_game = 0
// 				FinalNFLPlayer.Stat_run_yds_per_game = 0
// 				FinalNFLPlayer.Stat_def_int_per_game = 0
// 			} else {
// 				FinalNFLPlayer.Stat_def_sacks_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_sacks / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_qb_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_rec_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_run_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_def_int_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 			}
// 			FinalNFLPlayer.Stat_def_tackles = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tackles
// 			FinalNFLPlayer.Stat_def_tackles_for_loss = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tackles_for_loss
// 			FinalNFLPlayer.Stat_def_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tds
// 			FinalNFLPlayer.Stat_def_turnover_diff = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_turnover_diff
// 			FinalNFLPlayer.Stat_kick_ko_returns_td = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_ko_returns_td
// 			FinalNFLPlayer.Stat_punt_returns_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Punt_returns_tds
// 			FinalNFLPlayer.Stat_def_pa_per_game_16g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_16g
// 			FinalNFLPlayer.Stat_def_pa_per_game_4g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_4g
// 			FinalNFLPlayer.Stat_def_pa_per_game_1g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_1g

// 			// add in draftable unique id and final values
// 			FinalNFLPlayer.Draftable_uid = fmt.Sprint(s.PlayerDkId, "_", nfl_lookup, "_", s.Position, "_", "_", FinalNFLPlayer.Nfl_id, "_", FinalNFLPlayer.Salary)
// 			FinalNFLPlayer.Def_cupcake = nfl_players[fmt.Sprint(nfl_lookup)].Def_cupcake
// 			FinalNFLPlayer.Def_trend_up_bool = nfl_players[fmt.Sprint(nfl_lookup)].Def_trend_up_bool
// 			FinalNFLPlayer.Def_tough_bool = nfl_players[fmt.Sprint(nfl_lookup)].Def_tough_bool

// 			if implContains(FinalNFLPlayerKeys, FinalNFLPlayer.Draftable_uid) {
// 			} else if !FinalNFLPlayer.Inlineup {
// 			} else {
// 				FinalNFLPlayerKeys = append(FinalNFLPlayerKeys, FinalNFLPlayer.Draftable_uid)
// 				FinalNFLPlayers.Data = append(FinalNFLPlayers.Data, FinalNFLPlayer)
// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalNFLPlayers)
// 	}

// 	if (contextPosition == "nfl_players") && (contextService == "draftkings") && (strings.Contains(game_type, "Showdown")) {
// 		for _, s := range draftables.Draftables {
// 			FinalNFLPlayer = defaultNFLPlayer
// 			nfl_lookup = 0
// 			tmpTeamId = "None"
// 			nfl_lookup = Keys[fmt.Sprint(s.PlayerDkId)].PlayerID
// 			tmpTeamId = fmt.Sprint(nfl_players[fmt.Sprint(nfl_lookup)].Game_team_id)
// 			FinalNFLPlayer.Full_name = s.DisplayName
// 			FinalNFLPlayer.Nfl_id = fmt.Sprint(nfl_lookup)
// 			FinalNFLPlayer.Player_current_weight = nfl_players[fmt.Sprint(nfl_lookup)].Player_current_weight
// 			FinalNFLPlayer.Player_current_height = nfl_players[fmt.Sprint(nfl_lookup)].Player_current_height
// 			FinalNFLPlayer.Player_jersey_number = nfl_players[fmt.Sprint(nfl_lookup)].Player_jersey
// 			FinalNFLPlayer.Player_primary_position = nfl_players[fmt.Sprint(nfl_lookup)].Player_position
// 			FinalNFLPlayer.Team_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_team_name
// 			FinalNFLPlayer.Positions = strings.Split(s.Position, `/`)
// 			for i, p := range FinalNFLPlayer.Positions {
// 				FinalNFLPlayer.Positions[i] = strings.ToLower(p)
// 			}
// 			if s.RosterSlotId == 511 {
// 				FinalNFLPlayer.Positions = append(FinalNFLPlayer.Positions, "cpt")
// 				FinalNFLPlayer.Nfl_id = fmt.Sprint(nfl_lookup) + "_511"
// 			}
// 			if s.RosterSlotId == 512 {
// 				FinalNFLPlayer.Positions = append(FinalNFLPlayer.Positions, "flex")
// 				FinalNFLPlayer.Nfl_id = fmt.Sprint(nfl_lookup) + "_512"
// 			}
// 			FinalNFLPlayer.Inlineup = true
// 			for _, value := range s.PlayerGameAttributes {
// 				if value.Id == 100 {
// 					if value.Value == "false" {
// 						FinalNFLPlayer.Inlineup = false
// 					}
// 				}
// 			}
// 			if s.Position == "D" || s.Position == "DEF" {
// 				FinalNFLPlayer.Status = "None"
// 			} else {
// 				FinalNFLPlayer.Status = s.Status
// 			}
// 			FinalNFLPlayer.Salary = s.Salary
// 			FinalNFLPlayer.RosterSlotId = s.RosterSlotId
// 			FinalNFLPlayer.Lineup_selected = 0
// 			FinalNFLPlayer.ProjPoints = nfl_players[fmt.Sprint(nfl_lookup)].Dk_showdown_points_mean

// 			t_lessthan0 := 0
// 			t_one_nine := 0
// 			t_ten_nineteen := 0
// 			t_twt_twtnine := 0
// 			t_thr_thrnine := 0
// 			t_for_fornine := 0
// 			t_fft_fftnine := 0
// 			t_sixty_plus := 0

// 			for _, value := range nfl_players[fmt.Sprint(nfl_lookup)].Dk_showdown_points_list {
// 				if value <= 0 {
// 					t_lessthan0 += 1
// 				} else if value >= 0 && value < 10 {
// 					t_one_nine += 1
// 				} else if value >= 10 && value < 20 {
// 					t_ten_nineteen += 1
// 				} else if value >= 20 && value < 30 {
// 					t_twt_twtnine += 1
// 				} else if value >= 30 && value < 40 {
// 					t_thr_thrnine += 1
// 				} else if value >= 40 && value < 50 {
// 					t_for_fornine += 1
// 				} else if value >= 50 && value < 60 {
// 					t_fft_fftnine += 1
// 				} else if value >= 60 {
// 					t_sixty_plus += 1
// 				}
// 			}
// 			var tempPointsList []int
// 			tempPointsList = append(tempPointsList, t_lessthan0)
// 			tempPointsList = append(tempPointsList, t_one_nine)
// 			tempPointsList = append(tempPointsList, t_ten_nineteen)
// 			tempPointsList = append(tempPointsList, t_twt_twtnine)
// 			tempPointsList = append(tempPointsList, t_thr_thrnine)
// 			tempPointsList = append(tempPointsList, t_for_fornine)
// 			tempPointsList = append(tempPointsList, t_fft_fftnine)
// 			tempPointsList = append(tempPointsList, t_sixty_plus)
// 			FinalNFLPlayer.ProjPointsList = tempPointsList
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Game_is_dome {
// 				FinalNFLPlayer.Is_dome = 1
// 			} else {
// 				FinalNFLPlayer.Is_dome = 0
// 			}
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Game_is_home {
// 				FinalNFLPlayer.Is_home = 1
// 			} else {
// 				FinalNFLPlayer.Is_home = 0
// 			}
// 			FinalNFLPlayer.Temperature = nfl_players[fmt.Sprint(nfl_lookup)].Temperature
// 			FinalNFLPlayer.Humidity = nfl_players[fmt.Sprint(nfl_lookup)].Humidity
// 			FinalNFLPlayer.Wind_direction_norm = nfl_players[fmt.Sprint(nfl_lookup)].Wind_direction_norm
// 			FinalNFLPlayer.Wind_speed = nfl_players[fmt.Sprint(nfl_lookup)].Wind_speed
// 			FinalNFLPlayer.Weather_summary = nfl_players[fmt.Sprint(nfl_lookup)].Weather_summary
// 			FinalNFLPlayer.Game_abbr = nfl_players[fmt.Sprint(nfl_lookup)].Game_abbr
// 			FinalNFLPlayer.Game_date_unix = nfl_players[fmt.Sprint(nfl_lookup)].Game_date_unix
// 			FinalNFLPlayer.Game_gameDate = s.Competition.StartTime
// 			FinalNFLPlayer.Game_game_Pk = 1
// 			FinalNFLPlayer.Game_week = nfl_players[fmt.Sprint(nfl_lookup)].Game_week
// 			FinalNFLPlayer.Game_opponent = nfl_players[fmt.Sprint(nfl_lookup)].Game_opp_id
// 			FinalNFLPlayer.Game_opponent_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_opp_name
// 			FinalNFLPlayer.Game_player_oddsfactor = 1.0
// 			FinalNFLPlayer.Game_team_oddspoints = nfl_players[fmt.Sprint(nfl_lookup)].Game_points_for
// 			FinalNFLPlayer.Game_opponent_oddspoints = nfl_players[fmt.Sprint(nfl_lookup)].Game_points_against
// 			FinalNFLPlayer.Game_s_deviation = nfl_players[fmt.Sprint(nfl_lookup)].Dk_showdown_points_std
// 			FinalNFLPlayer.Game_sim_cume_games = nfl_players[fmt.Sprint(nfl_lookup)].Cume_sim_games
// 			FinalNFLPlayer.Games_sim_cume_points = nfl_players[fmt.Sprint(nfl_lookup)].Dk_showdown_cume_points
// 			FinalNFLPlayer.Game_venue = nfl_players[fmt.Sprint(nfl_lookup)].Game_venue_id
// 			FinalNFLPlayer.Game_venue_name = nfl_players[fmt.Sprint(nfl_lookup)].Game_venue_name

// 			// add in default statistics
// 			FinalNFLPlayer.Stat_gen_fum = 0
// 			FinalNFLPlayer.Stat_gen_fum_lost = 0
// 			FinalNFLPlayer.Stat_gen_gp = 0
// 			FinalNFLPlayer.Stat_kick_fg_att = 0
// 			FinalNFLPlayer.Stat_kick_fg_long = 0
// 			FinalNFLPlayer.Stat_kick_fg_made = 0
// 			FinalNFLPlayer.Stat_kick_fg_pct = 0
// 			FinalNFLPlayer.Stat_kick_xp_att = 0
// 			FinalNFLPlayer.Stat_kick_xp_made = 0
// 			FinalNFLPlayer.Stat_kick_xp_pct = 0
// 			FinalNFLPlayer.Stat_qb_att = 0
// 			FinalNFLPlayer.Stat_qb_cmp = 0
// 			FinalNFLPlayer.Stat_qb_cmp_pct = 0
// 			FinalNFLPlayer.Stat_qb_int = 0
// 			FinalNFLPlayer.Stat_qb_rtg = 0
// 			FinalNFLPlayer.Stat_qb_tds = 0
// 			FinalNFLPlayer.Stat_qb_yds = 0
// 			FinalNFLPlayer.Stat_qb_yds_per_cmp = 0
// 			FinalNFLPlayer.Stat_qb_yds_per_game = 0
// 			FinalNFLPlayer.Stat_rec_rec = 0
// 			FinalNFLPlayer.Stat_rec_tds = 0
// 			FinalNFLPlayer.Stat_rec_tgt = 0
// 			FinalNFLPlayer.Stat_rec_yds = 0
// 			FinalNFLPlayer.Stat_rec_yds_per_game = 0
// 			FinalNFLPlayer.Stat_rec_yds_per_rec = 0
// 			FinalNFLPlayer.Stat_run_att = 0
// 			FinalNFLPlayer.Stat_run_tds = 0
// 			FinalNFLPlayer.Stat_run_yds = 0
// 			FinalNFLPlayer.Stat_run_yds_per_att = 0
// 			FinalNFLPlayer.Stat_run_yds_per_game = 0
// 			FinalNFLPlayer.Stat_def_int = 0
// 			FinalNFLPlayer.Stat_def_int_tds = 0
// 			FinalNFLPlayer.Stat_def_int_per_game = 0
// 			FinalNFLPlayer.Stat_def_kicks_blocked = 0
// 			FinalNFLPlayer.Stat_def_pass_defended = 0
// 			FinalNFLPlayer.Stat_def_sacks = 0
// 			FinalNFLPlayer.Stat_def_sacks_per_game = 0
// 			FinalNFLPlayer.Stat_def_tackles = 0
// 			FinalNFLPlayer.Stat_def_tackles_for_loss = 0
// 			FinalNFLPlayer.Stat_def_tds = 0
// 			FinalNFLPlayer.Stat_def_turnover_diff = 0
// 			FinalNFLPlayer.Stat_kick_ko_returns_td = 0
// 			FinalNFLPlayer.Stat_punt_returns_tds = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_16g = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_4g = 0
// 			FinalNFLPlayer.Stat_def_pa_per_game_1g = 0

// 			// add in statistics if they exist
// 			FinalNFLPlayer.Stat_gen_fum = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_fum
// 			FinalNFLPlayer.Stat_gen_fum_lost = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_fum_lost
// 			FinalNFLPlayer.Stat_gen_gp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 			FinalNFLPlayer.Stat_kick_fg_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_att
// 			FinalNFLPlayer.Stat_kick_fg_long = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_long
// 			FinalNFLPlayer.Stat_kick_fg_made = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_made
// 			FinalNFLPlayer.Stat_kick_fg_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_fg_pct
// 			FinalNFLPlayer.Stat_kick_xp_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_att
// 			FinalNFLPlayer.Stat_kick_xp_made = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_made
// 			FinalNFLPlayer.Stat_kick_xp_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_xp_pct
// 			FinalNFLPlayer.Stat_qb_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_att
// 			FinalNFLPlayer.Stat_qb_cmp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_cmp
// 			FinalNFLPlayer.Stat_qb_cmp_pct = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_cmp_pct
// 			FinalNFLPlayer.Stat_qb_int = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_int
// 			FinalNFLPlayer.Stat_qb_rtg = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_rtg
// 			FinalNFLPlayer.Stat_qb_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_tds
// 			FinalNFLPlayer.Stat_qb_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds
// 			FinalNFLPlayer.Stat_qb_yds_per_cmp = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds_per_cmp
// 			FinalNFLPlayer.Stat_rec_rec = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_rec
// 			FinalNFLPlayer.Stat_rec_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_tds
// 			FinalNFLPlayer.Stat_rec_tgt = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_tgt
// 			FinalNFLPlayer.Stat_rec_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds
// 			FinalNFLPlayer.Stat_rec_yds_per_rec = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds_per_rec
// 			FinalNFLPlayer.Stat_run_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_att
// 			FinalNFLPlayer.Stat_run_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_tds
// 			FinalNFLPlayer.Stat_run_yds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds
// 			FinalNFLPlayer.Stat_run_yds_per_att = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds_per_att
// 			FinalNFLPlayer.Stat_def_int = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int
// 			FinalNFLPlayer.Stat_def_int_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int_tds
// 			FinalNFLPlayer.Stat_def_kicks_blocked = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_kicks_blocked
// 			FinalNFLPlayer.Stat_def_pass_defended = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_pass_defended
// 			FinalNFLPlayer.Stat_def_sacks = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_sacks
// 			if nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp == 0 {
// 				FinalNFLPlayer.Stat_def_sacks_per_game = 0
// 				FinalNFLPlayer.Stat_qb_yds_per_game = 0
// 				FinalNFLPlayer.Stat_rec_yds_per_game = 0
// 				FinalNFLPlayer.Stat_run_yds_per_game = 0
// 				FinalNFLPlayer.Stat_def_int_per_game = 0
// 			} else {
// 				FinalNFLPlayer.Stat_def_sacks_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_sacks / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_qb_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Qb_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_rec_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Rec_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_run_yds_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Run_yds / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 				FinalNFLPlayer.Stat_def_int_per_game = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_int / nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Gen_gp
// 			}
// 			FinalNFLPlayer.Stat_def_tackles = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tackles
// 			FinalNFLPlayer.Stat_def_tackles_for_loss = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tackles_for_loss
// 			FinalNFLPlayer.Stat_def_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_tds
// 			FinalNFLPlayer.Stat_def_turnover_diff = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Def_turnover_diff
// 			FinalNFLPlayer.Stat_kick_ko_returns_td = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Kick_ko_returns_td
// 			FinalNFLPlayer.Stat_punt_returns_tds = nfl_players[fmt.Sprint(nfl_lookup)].Stats_2023.Punt_returns_tds
// 			FinalNFLPlayer.Stat_def_pa_per_game_16g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_16g
// 			FinalNFLPlayer.Stat_def_pa_per_game_4g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_4g
// 			FinalNFLPlayer.Stat_def_pa_per_game_1g = nfl_players[fmt.Sprint(nfl_lookup)].Def_pa_per_game_1g

// 			// add in draftable unique id and final values
// 			FinalNFLPlayer.Draftable_uid = fmt.Sprint(s.PlayerDkId, "_", nfl_lookup, "_", s.Position, "_", "_", FinalNFLPlayer.Nfl_id, "_", FinalNFLPlayer.Salary)
// 			FinalNFLPlayer.Def_cupcake = nfl_players[fmt.Sprint(nfl_lookup)].Def_cupcake
// 			FinalNFLPlayer.Def_trend_up_bool = nfl_players[fmt.Sprint(nfl_lookup)].Def_trend_up_bool
// 			FinalNFLPlayer.Def_tough_bool = nfl_players[fmt.Sprint(nfl_lookup)].Def_tough_bool

// 			if implContains(FinalNFLPlayerKeys, FinalNFLPlayer.Draftable_uid) {
// 			} else if !FinalNFLPlayer.Inlineup {
// 			} else {
// 				FinalNFLPlayerKeys = append(FinalNFLPlayerKeys, FinalNFLPlayer.Draftable_uid)
// 				FinalNFLPlayers.Data = append(FinalNFLPlayers.Data, FinalNFLPlayer)
// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalNFLPlayers)
// 	}

// 	if (contextPosition == "pitchers") && (contextService == "draftkings") {
// 		for _, s := range draftables.Draftables {
// 			lookup = 0
// 			tmpTeamId = "None"
// 			if (strings.Contains(s.Position, `SP`)) || (strings.Contains(s.Position, `RP`)) {
// 				lookup = Keys[fmt.Sprint(s.PlayerDkId)].PlayerID
// 				tmpTeamId = pitchers[fmt.Sprint(lookup)].Team_id
// 				t, err := time.Parse(time.RFC3339, s.Competition.StartTime)
// 				if err != nil {
// 					fmt.Println(err.Error())
// 					panic(err)
// 				}
// 				FinalPitcher.ProbablePitcher = false
// 				FinalPitcher.LikelyPitcher = false
// 				for _, value := range s.PlayerGameAttributes {
// 					if value.Id == 1 {
// 						if value.Value == "true" {
// 							FinalPitcher.ProbablePitcher = true
// 						}
// 					} else if value.Id == 137 {
// 						if value.Value == "true" {
// 							FinalPitcher.LikelyPitcher = true
// 						}
// 					}
// 				}
// 				FinalPitcher.Status = s.Status
// 				FinalPitcher.Game_gameDate = ""
// 				FinalPitcher.ProjPoints = 0
// 				t_lessthan0 := 0
// 				t_one_nine := 0
// 				t_ten_nineteen := 0
// 				t_twt_twtnine := 0
// 				t_thr_thrnine := 0
// 				t_for_fornine := 0
// 				t_fft_fftnine := 0
// 				t_sixty_plus := 0
// 				FinalPitcher.Games_sim_cume_points = 0
// 				FinalPitcher.Game_game_Pk = 0
// 				FinalPitcher.Game_game_no = 0
// 				FinalPitcher.Game_opp_prob_phand = "R"
// 				FinalPitcher.Game_opp_prob_pitcher = 0
// 				FinalPitcher.Game_opponent = 0
// 				FinalPitcher.Game_opponent_name = ""
// 				FinalPitcher.Game_s_deviation = 0
// 				FinalPitcher.Game_sim_cume_games = 0
// 				FinalPitcher.Game_venue = 0
// 				FinalPitcher.Game_team_oddspoints = 0
// 				FinalPitcher.Game_player_oddsfactor = 0
// 				FinalPitcher.Game_opponent_oddspoints = 0
// 				for _, g_value := range pitchers[fmt.Sprint(lookup)].Games {
// 					gameTime, err := time.Parse(time.RFC3339, g_value.GameDate)
// 					if err != nil {
// 						fmt.Println(err.Error())
// 						panic(err)
// 					}
// 					gameTime = gameTime.Add(time.Hour)
// 					oddsGames := oddsDat[tmpTeamId].Games
// 					for ix, odds_values := range oddsGames {
// 						odds_commence_time := time.Unix(odds_values.Commence_time, 0)
// 						if gameTime.After(odds_commence_time) {
// 							FinalPitcher.Game_team_oddspoints = odds_values.Points
// 							FinalPitcher.Game_player_oddsfactor = odds_values.Points / TeamKeys[tmpTeamId].Avg_points
// 							FinalPitcher.Game_opponent_oddspoints = oddsDat[fmt.Sprint(g_value.Opponent)].Games[ix].Points
// 						}
// 						break
// 					}
// 					if gameTime.After(t) {
// 						FinalPitcher.Game_gameDate = g_value.GameDate
// 						if contextService == "draftkings" {
// 							FinalPitcher.ProjPoints = g_value.Draftkings_proj_points
// 							for _, value := range g_value.Dk_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Draftkings_cume_points
// 						} else if contextService == "fanduel" {
// 							FinalPitcher.ProjPoints = g_value.Fanduel_proj_points
// 							for _, value := range g_value.Fd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Fanduel_cume_points
// 						} else if contextService == "yahoo!" {
// 							FinalPitcher.ProjPoints = g_value.Yahoo_proj_points
// 							for _, value := range g_value.Yh_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Yahoo_cume_points
// 						} else if contextService == "superdraft" {
// 							FinalPitcher.ProjPoints = g_value.Superdraft_proj_points
// 							for _, value := range g_value.Sd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Superdraft_cume_points
// 						}
// 						FinalPitcher.Game_game_Pk = g_value.Game_Pk
// 						FinalPitcher.Game_game_no = g_value.Game_no
// 						FinalPitcher.Game_opp_prob_phand = g_value.Opp_prob_phand
// 						FinalPitcher.Game_opp_prob_pitcher = g_value.Opp_prob_pitcher
// 						FinalPitcher.Game_opponent = g_value.Opponent
// 						FinalPitcher.Game_opponent_name = g_value.Opponent_name
// 						FinalPitcher.Game_s_deviation = g_value.S_deviation
// 						FinalPitcher.Game_sim_cume_games = g_value.Sim_cume_games
// 						FinalPitcher.Game_venue = g_value.Venue
// 						break
// 					}
// 				}
// 				FinalPitcher.Date = fmt.Sprint(t)
// 				FinalPitcher.Full_name = s.DisplayName
// 				FinalPitcher.Mlb_id = lookup
// 				FinalPitcher.P_hand = pitchers[fmt.Sprint(lookup)].P_hand
// 				FinalPitcher.Player_dob = pitchers[fmt.Sprint(lookup)].Player_dob
// 				FinalPitcher.Team_name = pitchers[fmt.Sprint(lookup)].Team_name
// 				FinalPitcher.Positions = strings.Split(s.Position, `/`)
// 				for i, p := range FinalPitcher.Positions {
// 					FinalPitcher.Positions[i] = strings.ToLower(p)
// 				}
// 				if s.RosterSlotId == 573 {
// 					FinalPitcher.Positions = append(FinalPitcher.Positions, "cpt")
// 					FinalPitcher.Mlb_id = lookup * 1000
// 				}
// 				if s.RosterSlotId == 574 {
// 					FinalPitcher.Positions = append(FinalPitcher.Positions, "util")
// 					FinalPitcher.Mlb_id = lookup * 10000
// 				}
// 				FinalPitcher.Stat_avg = pitchers[fmt.Sprint(lookup)].Stats.Avg
// 				FinalPitcher.Stat_bb = pitchers[fmt.Sprint(lookup)].Stats.Bb
// 				FinalPitcher.Stat_cg = pitchers[fmt.Sprint(lookup)].Stats.Cg
// 				FinalPitcher.Stat_er = pitchers[fmt.Sprint(lookup)].Stats.Er
// 				FinalPitcher.Stat_era = pitchers[fmt.Sprint(lookup)].Stats.Era
// 				FinalPitcher.Stat_gamesplayed = pitchers[fmt.Sprint(lookup)].Stats.Gamesplayed
// 				FinalPitcher.Stat_gamesstarted = pitchers[fmt.Sprint(lookup)].Stats.Gamesstarted
// 				FinalPitcher.Stat_goao = pitchers[fmt.Sprint(lookup)].Stats.Goao
// 				FinalPitcher.Stat_inningspitched = pitchers[fmt.Sprint(lookup)].Stats.Inningspitched
// 				FinalPitcher.Stat_kp9 = pitchers[fmt.Sprint(lookup)].Stats.Kp9
// 				FinalPitcher.Stat_kpct = pitchers[fmt.Sprint(lookup)].Stats.Kpct
// 				FinalPitcher.Stat_l = pitchers[fmt.Sprint(lookup)].Stats.L
// 				FinalPitcher.Stat_ops = pitchers[fmt.Sprint(lookup)].Stats.Ops
// 				FinalPitcher.Stat_r = pitchers[fmt.Sprint(lookup)].Stats.R
// 				FinalPitcher.Stat_saves = pitchers[fmt.Sprint(lookup)].Stats.Saves
// 				FinalPitcher.Stat_sho = pitchers[fmt.Sprint(lookup)].Stats.Sho
// 				FinalPitcher.Stat_strikeouts = pitchers[fmt.Sprint(lookup)].Stats.Strikeouts
// 				FinalPitcher.Stat_w = pitchers[fmt.Sprint(lookup)].Stats.W
// 				FinalPitcher.Stat_whip = pitchers[fmt.Sprint(lookup)].Stats.Whip
// 				FinalPitcher.Salary = s.Salary
// 				FinalPitcher.RosterSlotId = s.RosterSlotId
// 				FinalPitcher.Lineup_selected = 0
// 				FinalPitcher.Draftable_uid = fmt.Sprint(s.PlayerDkId, "_", lookup, "_", s.Position, "_", "_", FinalPitcher.Mlb_id, "_", FinalPitcher.Salary)
// 				if implContains(FinalPitcherKeys, FinalPitcher.Draftable_uid) {
// 				} else if !FinalPitcher.ProbablePitcher {
// 				} else {
// 					FinalPitcherKeys = append(FinalPitcherKeys, FinalPitcher.Draftable_uid)
// 					FinalPitchers.Data = append(FinalPitchers.Data, FinalPitcher)
// 				}
// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalPitchers)
// 	}

// 	if (contextPosition == "batters") && (contextService == "draftkings") {
// 		for _, s := range draftables.Draftables {
// 			lookup = 0
// 			tmpTeamId = "None"
// 			if (!strings.Contains(s.Position, `SP`)) && (!strings.Contains(s.Position, `RP`)) {
// 				lookup = Keys[fmt.Sprint(s.PlayerDkId)].PlayerID
// 				tmpTeamId = batters[fmt.Sprint(lookup)].Team_id
// 				t, err := time.Parse(time.RFC3339, s.Competition.StartTime)
// 				if err != nil {
// 					fmt.Println(err.Error())
// 					panic(err)
// 				}
// 				FinalBatter.Inlineup = true
// 				FinalBatter.BattingOrder = ""
// 				for _, value := range s.PlayerGameAttributes {
// 					if value.Id == 100 {
// 						if value.Value == "false" {
// 							FinalBatter.Inlineup = false
// 						}
// 					}
// 					if value.Id == 99 {
// 						if value.Value != `-1` {
// 							FinalBatter.BattingOrder = value.Value
// 						}
// 					}
// 				}

// 				FinalBatter.Status = s.Status
// 				FinalBatter.Game_gameDate = ""
// 				FinalBatter.ProjPoints = 0
// 				t_lessthan0 := 0
// 				t_one_nine := 0
// 				t_ten_nineteen := 0
// 				t_twt_twtnine := 0
// 				t_thr_thrnine := 0
// 				t_for_fornine := 0
// 				t_fft_fftnine := 0
// 				t_sixty_plus := 0
// 				FinalBatter.Games_sim_cume_points = 0
// 				FinalBatter.Game_game_Pk = 0
// 				FinalBatter.Game_game_no = 0
// 				FinalBatter.Game_opp_prob_phand = "R"
// 				FinalBatter.Game_opp_prob_pitcher = 0
// 				FinalBatter.Game_opponent = 0
// 				FinalBatter.Game_opponent_name = ""
// 				FinalBatter.Game_s_deviation = 0
// 				FinalBatter.Game_sim_cume_games = 0
// 				FinalBatter.Game_venue = 0
// 				FinalBatter.Hot_streak = false
// 				FinalBatter.Humidity = 0
// 				FinalBatter.Is_dome = 0
// 				FinalBatter.Is_home = 0
// 				FinalBatter.Day_night = "night"
// 				FinalBatter.Ops_162g = 0
// 				FinalBatter.Ops_30g = 0
// 				FinalBatter.Ops_7g = 0
// 				FinalBatter.Pmatchup_aogo_30g = 0
// 				FinalBatter.Pmatchup_era_15g = 0
// 				FinalBatter.Pmatchup_era_30g = 0
// 				FinalBatter.Temperature = 0
// 				FinalBatter.Trend_up = false
// 				FinalBatter.Weather_summary = "clear"
// 				FinalBatter.Wind_direction_norm = 0
// 				FinalBatter.Wind_speed = 0
// 				FinalBatter.Game_team_oddspoints = 0
// 				FinalBatter.Game_player_oddsfactor = 0
// 				FinalBatter.Game_opponent_oddspoints = 0
// 				for _, g_value := range batters[fmt.Sprint(lookup)].Games {
// 					gameTime, err := time.Parse(time.RFC3339, g_value.GameDate)
// 					if err != nil {
// 						fmt.Println(err.Error())
// 						panic(err)
// 					}
// 					gameTime = gameTime.Add(time.Hour)
// 					oddsGames := oddsDat[tmpTeamId].Games
// 					for ix, odds_values := range oddsGames {
// 						odds_commence_time := time.Unix(odds_values.Commence_time, 0)
// 						if gameTime.After(odds_commence_time) {
// 							FinalBatter.Game_team_oddspoints = odds_values.Points
// 							FinalBatter.Game_player_oddsfactor = odds_values.Points / TeamKeys[tmpTeamId].Avg_points
// 							FinalBatter.Game_opponent_oddspoints = oddsDat[fmt.Sprint(g_value.Opponent)].Games[ix].Points
// 						}
// 						break
// 					}
// 					if gameTime.After(t) {
// 						FinalBatter.Game_gameDate = g_value.GameDate
// 						if contextService == "draftkings" {
// 							FinalBatter.ProjPoints = g_value.Draftkings_proj_points
// 							for _, value := range g_value.Dk_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Draftkings_cume_points
// 						} else if contextService == "fanduel" {
// 							FinalBatter.ProjPoints = g_value.Fanduel_proj_points
// 							for _, value := range g_value.Fd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Fanduel_cume_points
// 						} else if contextService == "yahoo!" {
// 							FinalBatter.ProjPoints = g_value.Yahoo_proj_points
// 							for _, value := range g_value.Yh_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Yahoo_cume_points
// 						} else if contextService == "superdraft" {
// 							FinalBatter.ProjPoints = g_value.Superdraft_proj_points
// 							for _, value := range g_value.Sd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Superdraft_cume_points
// 						}
// 						FinalBatter.Game_game_Pk = g_value.Game_Pk
// 						FinalBatter.Game_game_no = g_value.Game_no
// 						FinalBatter.Game_opp_prob_phand = g_value.Opp_prob_phand
// 						FinalBatter.Game_opp_prob_pitcher = g_value.Opp_prob_pitcher
// 						FinalBatter.Game_opponent = g_value.Opponent
// 						FinalBatter.Game_opponent_name = g_value.Opponent_name
// 						FinalBatter.Game_player_pfactor = g_value.Player_pfactor
// 						FinalBatter.Game_player_splt_ops = g_value.Player_splt_ops
// 						FinalBatter.Game_s_deviation = g_value.S_deviation
// 						FinalBatter.Game_sim_cume_games = g_value.Sim_cume_games
// 						FinalBatter.Game_venue = g_value.Venue
// 						FinalBatter.Hot_streak = g_value.Hot_streak
// 						FinalBatter.Humidity = g_value.Humidity
// 						FinalBatter.Is_dome = g_value.Is_dome
// 						FinalBatter.Is_home = g_value.Is_home
// 						FinalBatter.Day_night = g_value.Day_night
// 						FinalBatter.Ops_162g = g_value.Ops_162g
// 						FinalBatter.Ops_30g = g_value.Ops_30g
// 						FinalBatter.Ops_7g = g_value.Ops_7g
// 						FinalBatter.Pmatchup_aogo_30g = g_value.Pmatchup_aogo_30g
// 						FinalBatter.Pmatchup_era_15g = g_value.Pmatchup_era_15g
// 						FinalBatter.Pmatchup_era_30g = g_value.Pmatchup_era_30g
// 						FinalBatter.Temperature = g_value.Temperature
// 						FinalBatter.Trend_up = g_value.Trend_up
// 						FinalBatter.Weather_summary = g_value.Weather_summary
// 						FinalBatter.Wind_direction_norm = g_value.Wind_direction_norm
// 						FinalBatter.Wind_speed = g_value.Wind_speed
// 						break
// 					}
// 				}
// 				FinalBatter.Bat_side = batters[fmt.Sprint(lookup)].Bat_side
// 				FinalBatter.Date = fmt.Sprint(t)
// 				FinalBatter.Full_name = s.DisplayName
// 				FinalBatter.Mlb_id = lookup
// 				FinalBatter.Player_dob = batters[fmt.Sprint(lookup)].Player_dob
// 				FinalBatter.Team_name = batters[fmt.Sprint(lookup)].Team_name
// 				FinalBatter.Positions = strings.Split(s.Position, `/`)
// 				for i, p := range FinalBatter.Positions {
// 					FinalBatter.Positions[i] = strings.ToLower(p)
// 				}
// 				FinalBatter.Stat_ab = batters[fmt.Sprint(lookup)].Stats.Ab
// 				FinalBatter.Stat_avg = batters[fmt.Sprint(lookup)].Stats.Avg
// 				FinalBatter.Stat_bb = batters[fmt.Sprint(lookup)].Stats.Bb
// 				FinalBatter.Stat_cs = batters[fmt.Sprint(lookup)].Stats.Cs
// 				FinalBatter.Stat_doubles = batters[fmt.Sprint(lookup)].Stats.Doubles
// 				FinalBatter.Stat_g = batters[fmt.Sprint(lookup)].Stats.G
// 				FinalBatter.Stat_hbp = batters[fmt.Sprint(lookup)].Stats.Hbp
// 				FinalBatter.Stat_hits = batters[fmt.Sprint(lookup)].Stats.Hits
// 				FinalBatter.Stat_hr = batters[fmt.Sprint(lookup)].Stats.Hr
// 				FinalBatter.Stat_ibb = batters[fmt.Sprint(lookup)].Stats.Ibb
// 				FinalBatter.Stat_obp = batters[fmt.Sprint(lookup)].Stats.Obp
// 				FinalBatter.Stat_ops = batters[fmt.Sprint(lookup)].Stats.Ops
// 				FinalBatter.Stat_rbi = batters[fmt.Sprint(lookup)].Stats.Rbi
// 				FinalBatter.Stat_runs = batters[fmt.Sprint(lookup)].Stats.Runs
// 				FinalBatter.Stat_sb = batters[fmt.Sprint(lookup)].Stats.Sb
// 				FinalBatter.Stat_single = batters[fmt.Sprint(lookup)].Stats.Single
// 				FinalBatter.Stat_slg = batters[fmt.Sprint(lookup)].Stats.Slg
// 				FinalBatter.Stat_strikeouts = batters[fmt.Sprint(lookup)].Stats.Strikeouts
// 				FinalBatter.Stat_triples = batters[fmt.Sprint(lookup)].Stats.Triples
// 				FinalBatter.Salary = s.Salary
// 				FinalBatter.RosterSlotId = s.RosterSlotId
// 				FinalBatter.Lineup_selected = 0
// 				FinalBatter.Draftable_uid = fmt.Sprint(s.PlayerDkId, "_", lookup, "_", s.Position, "_", "_", FinalBatter.Mlb_id, "_", FinalBatter.Salary)
// 				if implContains(FinalBatterKeys, FinalBatter.Draftable_uid) {
// 				} else if !FinalBatter.Inlineup {
// 				} else {
// 					FinalBatterKeys = append(FinalBatterKeys, FinalBatter.Draftable_uid)
// 					FinalBatters.Data = append(FinalBatters.Data, FinalBatter)
// 				}

// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalBatters)
// 	}

// 	if (contextPosition == "pitchers") && (contextService == "fanduel") {
// 		for _, s := range players.Players {
// 			lookup = 0
// 			tmpTeamId = "None"
// 			player_injured = false
// 			player_injured = s.Injured
// 			player_injury_status = "None"
// 			if strings.Contains(s.Position, `P`) {
// 				FDidLookup := strings.Split(s.Id, "-")[1]
// 				lookup = Keys[fmt.Sprint(FDidLookup)].PlayerID
// 				tmpTeamId = pitchers[fmt.Sprint(lookup)].Team_id
// 				t := time.Unix(contextTimeInt/1000, 0).UTC()
// 				FinalPitcher.ProbablePitcher = s.Probable_pitcher
// 				FinalPitcher.LikelyPitcher = false
// 				if player_injured {
// 					player_injury_status = s.Injury_status
// 				}
// 				FinalPitcher.Status = player_injury_status
// 				FinalPitcher.Game_gameDate = ""
// 				FinalPitcher.ProjPoints = 0
// 				t_lessthan0 := 0
// 				t_one_nine := 0
// 				t_ten_nineteen := 0
// 				t_twt_twtnine := 0
// 				t_thr_thrnine := 0
// 				t_for_fornine := 0
// 				t_fft_fftnine := 0
// 				t_sixty_plus := 0
// 				FinalPitcher.Games_sim_cume_points = 0
// 				FinalPitcher.Game_game_Pk = 0
// 				FinalPitcher.Game_game_no = 0
// 				FinalPitcher.Game_opp_prob_phand = "R"
// 				FinalPitcher.Game_opp_prob_pitcher = 0
// 				FinalPitcher.Game_opponent = 0
// 				FinalPitcher.Game_opponent_name = ""
// 				FinalPitcher.Game_s_deviation = 0
// 				FinalPitcher.Game_sim_cume_games = 0
// 				FinalPitcher.Game_venue = 0
// 				FinalPitcher.Game_team_oddspoints = 0
// 				FinalPitcher.Game_player_oddsfactor = 0
// 				FinalPitcher.Game_team_oddspoints = 0
// 				FinalPitcher.Game_opponent_oddspoints = 0
// 				for _, g_value := range pitchers[fmt.Sprint(lookup)].Games {
// 					gameTime, err := time.Parse(time.RFC3339, g_value.GameDate)
// 					if err != nil {
// 						fmt.Println(err.Error())
// 						panic(err)
// 					}
// 					gameTime = gameTime.Add(time.Hour)
// 					oddsGames := oddsDat[tmpTeamId].Games
// 					for ix, odds_values := range oddsGames {
// 						odds_commence_time := time.Unix(odds_values.Commence_time, 0)
// 						if gameTime.After(odds_commence_time) {
// 							FinalPitcher.Game_team_oddspoints = odds_values.Points
// 							FinalPitcher.Game_player_oddsfactor = odds_values.Points / TeamKeys[tmpTeamId].Avg_points
// 							FinalPitcher.Game_opponent_oddspoints = oddsDat[fmt.Sprint(g_value.Opponent)].Games[ix].Points
// 						}
// 						break
// 					}
// 					if gameTime.After(t) {
// 						FinalPitcher.Game_gameDate = g_value.GameDate
// 						if contextService == "draftkings" {
// 							FinalPitcher.ProjPoints = g_value.Draftkings_proj_points
// 							for _, value := range g_value.Dk_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Draftkings_cume_points
// 						} else if contextService == "fanduel" {
// 							FinalPitcher.ProjPoints = g_value.Fanduel_proj_points
// 							for _, value := range g_value.Fd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Fanduel_cume_points
// 						} else if contextService == "yahoo!" {
// 							FinalPitcher.ProjPoints = g_value.Yahoo_proj_points
// 							for _, value := range g_value.Yh_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Yahoo_cume_points
// 						} else if contextService == "superdraft" {
// 							FinalPitcher.ProjPoints = g_value.Superdraft_proj_points
// 							for _, value := range g_value.Sd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalPitcher.ProjPointsList = tempPointsList
// 							FinalPitcher.Games_sim_cume_points = g_value.Superdraft_cume_points
// 						}
// 						FinalPitcher.Game_game_Pk = g_value.Game_Pk
// 						FinalPitcher.Game_game_no = g_value.Game_no
// 						FinalPitcher.Game_opp_prob_phand = g_value.Opp_prob_phand
// 						FinalPitcher.Game_opp_prob_pitcher = g_value.Opp_prob_pitcher
// 						FinalPitcher.Game_opponent = g_value.Opponent
// 						FinalPitcher.Game_opponent_name = g_value.Opponent_name
// 						FinalPitcher.Game_s_deviation = g_value.S_deviation
// 						FinalPitcher.Game_sim_cume_games = g_value.Sim_cume_games
// 						FinalPitcher.Game_venue = g_value.Venue
// 						break
// 					}
// 				}
// 				FinalPitcher.Date = fmt.Sprint(t)
// 				FinalPitcher.Full_name = s.First_name + " " + s.Last_name
// 				FinalPitcher.Mlb_id = lookup
// 				FinalPitcher.P_hand = pitchers[fmt.Sprint(lookup)].P_hand
// 				FinalPitcher.Player_dob = pitchers[fmt.Sprint(lookup)].Player_dob
// 				FinalPitcher.Team_name = pitchers[fmt.Sprint(lookup)].Team_name
// 				FinalPitcher.Positions = s.Positions
// 				for i, p := range FinalPitcher.Positions {
// 					FinalPitcher.Positions[i] = strings.ToLower(p)
// 				}
// 				FinalPitcher.Stat_avg = pitchers[fmt.Sprint(lookup)].Stats.Avg
// 				FinalPitcher.Stat_bb = pitchers[fmt.Sprint(lookup)].Stats.Bb
// 				FinalPitcher.Stat_cg = pitchers[fmt.Sprint(lookup)].Stats.Cg
// 				FinalPitcher.Stat_er = pitchers[fmt.Sprint(lookup)].Stats.Er
// 				FinalPitcher.Stat_era = pitchers[fmt.Sprint(lookup)].Stats.Era
// 				FinalPitcher.Stat_gamesplayed = pitchers[fmt.Sprint(lookup)].Stats.Gamesplayed
// 				FinalPitcher.Stat_gamesstarted = pitchers[fmt.Sprint(lookup)].Stats.Gamesstarted
// 				FinalPitcher.Stat_goao = pitchers[fmt.Sprint(lookup)].Stats.Goao
// 				FinalPitcher.Stat_inningspitched = pitchers[fmt.Sprint(lookup)].Stats.Inningspitched
// 				FinalPitcher.Stat_kp9 = pitchers[fmt.Sprint(lookup)].Stats.Kp9
// 				FinalPitcher.Stat_kpct = pitchers[fmt.Sprint(lookup)].Stats.Kpct
// 				FinalPitcher.Stat_l = pitchers[fmt.Sprint(lookup)].Stats.L
// 				FinalPitcher.Stat_ops = pitchers[fmt.Sprint(lookup)].Stats.Ops
// 				FinalPitcher.Stat_r = pitchers[fmt.Sprint(lookup)].Stats.R
// 				FinalPitcher.Stat_saves = pitchers[fmt.Sprint(lookup)].Stats.Saves
// 				FinalPitcher.Stat_sho = pitchers[fmt.Sprint(lookup)].Stats.Sho
// 				FinalPitcher.Stat_strikeouts = pitchers[fmt.Sprint(lookup)].Stats.Strikeouts
// 				FinalPitcher.Stat_w = pitchers[fmt.Sprint(lookup)].Stats.W
// 				FinalPitcher.Stat_whip = pitchers[fmt.Sprint(lookup)].Stats.Whip
// 				FinalPitcher.Salary = s.Salary
// 				FinalPitcher.RosterSlotId = 0
// 				FinalPitcher.Lineup_selected = 0
// 				FinalPitcher.Draftable_uid = fmt.Sprint(FDidLookup, "_", lookup, "_", s.Position, "_", "_", FinalPitcher.Mlb_id, "_", FinalPitcher.Salary)
// 				if implContains(FinalPitcherKeys, FinalPitcher.Draftable_uid) {
// 				} else if !FinalPitcher.ProbablePitcher {
// 				} else {
// 					FinalPitcherKeys = append(FinalPitcherKeys, FinalPitcher.Draftable_uid)
// 					FinalPitchers.Data = append(FinalPitchers.Data, FinalPitcher)
// 				}
// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalPitchers)
// 	}

// 	if (contextPosition == "batters") && (contextService == "fanduel") {
// 		for _, s := range players.Players {
// 			lookup = 0
// 			tmpTeamId = "None"
// 			player_injured = false
// 			player_injured = s.Injured
// 			player_injury_status = "None"
// 			if !strings.Contains(s.Position, `P`) {
// 				FDidLookup := strings.Split(s.Id, "-")[1]
// 				lookup = Keys[fmt.Sprint(FDidLookup)].PlayerID
// 				tmpTeamId = batters[fmt.Sprint(lookup)].Team_id
// 				t := time.Unix(contextTimeInt/1000, 0).UTC()
// 				if err != nil {
// 					fmt.Println(err.Error())
// 					panic(err)
// 				}
// 				FinalBatter.Inlineup = s.Draftable
// 				FinalBatter.BattingOrder = fmt.Sprint(s.Projected_starting_order)
// 				if player_injured {
// 					player_injury_status = s.Injury_status
// 				}
// 				FinalBatter.Status = player_injury_status
// 				FinalBatter.Game_gameDate = ""
// 				FinalBatter.ProjPoints = 0
// 				t_lessthan0 := 0
// 				t_one_nine := 0
// 				t_ten_nineteen := 0
// 				t_twt_twtnine := 0
// 				t_thr_thrnine := 0
// 				t_for_fornine := 0
// 				t_fft_fftnine := 0
// 				t_sixty_plus := 0
// 				FinalBatter.Games_sim_cume_points = 0
// 				FinalBatter.Game_game_Pk = 0
// 				FinalBatter.Game_game_no = 0
// 				FinalBatter.Game_opp_prob_phand = "R"
// 				FinalBatter.Game_opp_prob_pitcher = 0
// 				FinalBatter.Game_opponent = 0
// 				FinalBatter.Game_opponent_name = ""
// 				FinalBatter.Game_s_deviation = 0
// 				FinalBatter.Game_sim_cume_games = 0
// 				FinalBatter.Game_venue = 0
// 				FinalBatter.Hot_streak = false
// 				FinalBatter.Humidity = 0
// 				FinalBatter.Is_dome = 0
// 				FinalBatter.Is_home = 0
// 				FinalBatter.Day_night = "night"
// 				FinalBatter.Ops_162g = 0
// 				FinalBatter.Ops_30g = 0
// 				FinalBatter.Ops_7g = 0
// 				FinalBatter.Pmatchup_aogo_30g = 0
// 				FinalBatter.Pmatchup_era_15g = 0
// 				FinalBatter.Pmatchup_era_30g = 0
// 				FinalBatter.Temperature = 0
// 				FinalBatter.Trend_up = false
// 				FinalBatter.Weather_summary = "clear"
// 				FinalBatter.Wind_direction_norm = 0
// 				FinalBatter.Wind_speed = 0
// 				FinalBatter.Game_team_oddspoints = 0
// 				FinalBatter.Game_player_oddsfactor = 0
// 				FinalBatter.Game_opponent_oddspoints = 0
// 				for _, g_value := range batters[fmt.Sprint(lookup)].Games {
// 					gameTime, err := time.Parse(time.RFC3339, g_value.GameDate)
// 					if err != nil {
// 						fmt.Println(err.Error())
// 						panic(err)
// 					}
// 					gameTime = gameTime.Add(time.Hour)
// 					oddsGames := oddsDat[tmpTeamId].Games
// 					for ix, odds_values := range oddsGames {
// 						odds_commence_time := time.Unix(odds_values.Commence_time, 0)
// 						if gameTime.After(odds_commence_time) {
// 							FinalBatter.Game_team_oddspoints = odds_values.Points
// 							FinalBatter.Game_player_oddsfactor = odds_values.Points / TeamKeys[tmpTeamId].Avg_points
// 							FinalBatter.Game_opponent_oddspoints = oddsDat[fmt.Sprint(g_value.Opponent)].Games[ix].Points
// 						}
// 						break
// 					}
// 					if gameTime.After(t) {
// 						FinalBatter.Game_gameDate = g_value.GameDate
// 						if contextService == "draftkings" {
// 							FinalBatter.ProjPoints = g_value.Draftkings_proj_points
// 							for _, value := range g_value.Dk_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Draftkings_cume_points
// 						} else if contextService == "fanduel" {
// 							FinalBatter.ProjPoints = g_value.Fanduel_proj_points
// 							for _, value := range g_value.Fd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Fanduel_cume_points
// 						} else if contextService == "yahoo!" {
// 							FinalBatter.ProjPoints = g_value.Yahoo_proj_points
// 							for _, value := range g_value.Yh_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Yahoo_cume_points
// 						} else if contextService == "superdraft" {
// 							FinalBatter.ProjPoints = g_value.Superdraft_proj_points
// 							for _, value := range g_value.Sd_points_list {
// 								if value <= 0 {
// 									t_lessthan0 += 1
// 								} else if value >= 0 && value < 10 {
// 									t_one_nine += 1
// 								} else if value >= 10 && value < 20 {
// 									t_ten_nineteen += 1
// 								} else if value >= 20 && value < 30 {
// 									t_twt_twtnine += 1
// 								} else if value >= 30 && value < 40 {
// 									t_thr_thrnine += 1
// 								} else if value >= 40 && value < 50 {
// 									t_for_fornine += 1
// 								} else if value >= 50 && value < 60 {
// 									t_fft_fftnine += 1
// 								} else if value >= 60 {
// 									t_sixty_plus += 1
// 								}
// 							}
// 							var tempPointsList []int
// 							tempPointsList = append(tempPointsList, t_lessthan0)
// 							tempPointsList = append(tempPointsList, t_one_nine)
// 							tempPointsList = append(tempPointsList, t_ten_nineteen)
// 							tempPointsList = append(tempPointsList, t_twt_twtnine)
// 							tempPointsList = append(tempPointsList, t_thr_thrnine)
// 							tempPointsList = append(tempPointsList, t_for_fornine)
// 							tempPointsList = append(tempPointsList, t_fft_fftnine)
// 							tempPointsList = append(tempPointsList, t_sixty_plus)
// 							FinalBatter.ProjPointsList = tempPointsList
// 							FinalBatter.Games_sim_cume_points = g_value.Superdraft_cume_points
// 						}
// 						FinalBatter.Game_game_Pk = g_value.Game_Pk
// 						FinalBatter.Game_game_no = g_value.Game_no
// 						FinalBatter.Game_opp_prob_phand = g_value.Opp_prob_phand
// 						FinalBatter.Game_opp_prob_pitcher = g_value.Opp_prob_pitcher
// 						FinalBatter.Game_opponent = g_value.Opponent
// 						FinalBatter.Game_opponent_name = g_value.Opponent_name
// 						FinalBatter.Game_player_pfactor = g_value.Player_pfactor
// 						FinalBatter.Game_player_splt_ops = g_value.Player_splt_ops
// 						FinalBatter.Game_s_deviation = g_value.S_deviation
// 						FinalBatter.Game_sim_cume_games = g_value.Sim_cume_games
// 						FinalBatter.Game_venue = g_value.Venue
// 						FinalBatter.Hot_streak = g_value.Hot_streak
// 						FinalBatter.Humidity = g_value.Humidity
// 						FinalBatter.Is_dome = g_value.Is_dome
// 						FinalBatter.Is_home = g_value.Is_home
// 						FinalBatter.Day_night = g_value.Day_night
// 						FinalBatter.Ops_162g = g_value.Ops_162g
// 						FinalBatter.Ops_30g = g_value.Ops_30g
// 						FinalBatter.Ops_7g = g_value.Ops_7g
// 						FinalBatter.Pmatchup_aogo_30g = g_value.Pmatchup_aogo_30g
// 						FinalBatter.Pmatchup_era_15g = g_value.Pmatchup_era_15g
// 						FinalBatter.Pmatchup_era_30g = g_value.Pmatchup_era_30g
// 						FinalBatter.Temperature = g_value.Temperature
// 						FinalBatter.Trend_up = g_value.Trend_up
// 						FinalBatter.Weather_summary = g_value.Weather_summary
// 						FinalBatter.Wind_direction_norm = g_value.Wind_direction_norm
// 						FinalBatter.Wind_speed = g_value.Wind_speed
// 						break
// 					}
// 				}
// 				FinalBatter.Bat_side = batters[fmt.Sprint(lookup)].Bat_side
// 				FinalBatter.Date = fmt.Sprint(t)
// 				FinalBatter.Full_name = s.First_name + " " + s.Last_name
// 				FinalBatter.Mlb_id = lookup
// 				FinalBatter.Player_dob = batters[fmt.Sprint(lookup)].Player_dob
// 				FinalBatter.Team_name = batters[fmt.Sprint(lookup)].Team_name
// 				FinalBatter.Positions = s.Positions
// 				for i, p := range FinalBatter.Positions {
// 					FinalBatter.Positions[i] = strings.ToLower(p)
// 				}
// 				FinalBatter.Positions = append(FinalBatter.Positions, "util")
// 				FinalBatter.Stat_ab = batters[fmt.Sprint(lookup)].Stats.Ab
// 				FinalBatter.Stat_avg = batters[fmt.Sprint(lookup)].Stats.Avg
// 				FinalBatter.Stat_bb = batters[fmt.Sprint(lookup)].Stats.Bb
// 				FinalBatter.Stat_cs = batters[fmt.Sprint(lookup)].Stats.Cs
// 				FinalBatter.Stat_doubles = batters[fmt.Sprint(lookup)].Stats.Doubles
// 				FinalBatter.Stat_g = batters[fmt.Sprint(lookup)].Stats.G
// 				FinalBatter.Stat_hbp = batters[fmt.Sprint(lookup)].Stats.Hbp
// 				FinalBatter.Stat_hits = batters[fmt.Sprint(lookup)].Stats.Hits
// 				FinalBatter.Stat_hr = batters[fmt.Sprint(lookup)].Stats.Hr
// 				FinalBatter.Stat_ibb = batters[fmt.Sprint(lookup)].Stats.Ibb
// 				FinalBatter.Stat_obp = batters[fmt.Sprint(lookup)].Stats.Obp
// 				FinalBatter.Stat_ops = batters[fmt.Sprint(lookup)].Stats.Ops
// 				FinalBatter.Stat_rbi = batters[fmt.Sprint(lookup)].Stats.Rbi
// 				FinalBatter.Stat_runs = batters[fmt.Sprint(lookup)].Stats.Runs
// 				FinalBatter.Stat_sb = batters[fmt.Sprint(lookup)].Stats.Sb
// 				FinalBatter.Stat_single = batters[fmt.Sprint(lookup)].Stats.Single
// 				FinalBatter.Stat_slg = batters[fmt.Sprint(lookup)].Stats.Slg
// 				FinalBatter.Stat_strikeouts = batters[fmt.Sprint(lookup)].Stats.Strikeouts
// 				FinalBatter.Stat_triples = batters[fmt.Sprint(lookup)].Stats.Triples
// 				FinalBatter.Salary = s.Salary
// 				FinalBatter.RosterSlotId = 0
// 				FinalBatter.Lineup_selected = 0
// 				FinalBatter.Draftable_uid = fmt.Sprint(FDidLookup, "_", lookup, "_", s.Position, "_", "_", FinalBatter.Mlb_id, "_", FinalBatter.Salary)
// 				if implContains(FinalBatterKeys, FinalBatter.Draftable_uid) {
// 				} else if !FinalBatter.Inlineup {
// 				} else {
// 					FinalBatterKeys = append(FinalBatterKeys, FinalBatter.Draftable_uid)
// 					FinalBatters.Data = append(FinalBatters.Data, FinalBatter)
// 				}

// 			}

// 		}
// 		c.IndentedJSON(http.StatusOK, FinalBatters)
// 	}
// }

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
