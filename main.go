package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/legolord208/stdutil"
)

var botID string
var avatarURL string

const format = "03:04 PM"
const format24 = "15:04"
const help = `
**Welcome to TimeyWimey!**
This is the bot that manages your timezones... for you.

Specify your timezone:
` + "`@TimeyWimey timezone <timezone> [24h]`" + `
Get time of user:
` + "`@TimeyWimey timefor @user1 @user2 et.c`" + `
Get time of user at specific time:
` + "`@TimeyWimey timeat <time> @users`" + `
Get the timezone difference:
` + "`@TimeyWimey timediff @users`" + `
Examples:
` + "`@TimeyWimey timezone europe/stockholm`" + `
Saved timezone "Europe/Stockholm" for LEGOlord208. Current time is 06:66 AM
` + "`@TimeyWimey timefor`" + `
Current time for LEGOlord208 is 06:66 AM.
` + "`@TimeyWimey timefor @test @LEGOlord208`" + `
Current time for test is 07:66 AM.
Current time for LEGOlord208 is 06:66 AM.
` + "`@TimeyWimey timeat 7PM @LEGOlord208`" + `
7PM your time is 08:00 PM for test.
`
const about = `
Hello! I'm TimeyWimey.
I take care of the timezone trouble.

Do ` + "`@TimeyWimey help`" + ` for some help.

I'm written in Go. Using, well, discordgo.
Ok, have fun! Bai bai!
`

type user struct {
	TimeZone string
	Is24h    bool `json:",omitempty"`
}

var timezones = make(map[string]*user)
var cache = make(map[string]*time.Location)

var (
	mutexTimezones sync.RWMutex
	mutexCache     sync.RWMutex
)

var regexMentions = regexp.MustCompile("\\s*<@!?[0-9]+>\\s*")

func main() {
	args := os.Args[1:]

	stdutil.EventPrePrintError = append(stdutil.EventPrePrintError, func(full string, msg string, err error) bool {
		return err != nil && isPermission(err)
	})

	if len(args) < 1 {
		fmt.Println("No token supplied in arguments.")
		return
	}
	token := args[0]

	fmt.Println("Loading...")

	file, err := os.Open("timeywimey.json")
	if err != nil {
		if !os.IsNotExist(err) {
			stdutil.PrintErr("Couldn't read file", err)
			return
		}
	} else {
		mutexTimezones.Lock()
		err = json.NewDecoder(file).Decode(&timezones)
		mutexTimezones.Unlock()
		file.Close()
		if err != nil {
			stdutil.PrintErr("Could not load JSON", err)
			return
		}
	}

	fmt.Println("Starting...")

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		stdutil.PrintErr("Could not create Discord session", err)
		return
	}

	user, err := session.User("@me")
	if err != nil {
		stdutil.PrintErr("Could not query user", err)
		return
	}
	botID = user.ID
	avatarURL = discordgo.EndpointUserAvatar(user.ID, user.Avatar)

	session.AddHandler(messageCreate)
	session.AddHandler(messageUpdate)

	err = session.Open()
	if err != nil {
		stdutil.PrintErr("Could not start Discord session", err)
		return
	}

	fmt.Println("Started!")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	<-interrupt
	fmt.Println("\nExiting")
	session.Close()
}

func messageCreate(session *discordgo.Session, e *discordgo.MessageCreate) {
	message(session, e.Message)
}
func messageUpdate(session *discordgo.Session, e *discordgo.MessageUpdate) {
	message(session, e.Message)
}
func message(session *discordgo.Session, e *discordgo.Message) {
	if e.Author == nil || e.Author.Bot {
		return
	}
	msg := strings.ToLower(strings.TrimSpace(e.Content))

	if msg == "" {
		return
	}

	index := -1
	for i, mention := range e.Mentions {
		if mention.ID == botID {
			index = i
		}
	}
	if index < 0 {
		return
	}
	e.Mentions = append(e.Mentions[:index], e.Mentions[index+1:]...)

	msg = regexMentions.ReplaceAllString(msg, "")
	msg = strings.TrimSpace(msg)

	parts := strings.Fields(msg)
	if len(parts) <= 0 {
		return
	}
	cmd := parts[0]
	args := parts[1:]

	if cmd == "ping" {
		now := time.Now().UTC()

		timestamp := e.EditedTimestamp
		if timestamp == "" {
			timestamp = e.Timestamp
		}

		t, err := timestamp.Parse()
		if err != nil {
			stdutil.PrintErr("Couldn't parse timestamp", err)
			return
		}

		diff := now.Sub(t)
		sendMessage(session, e.ChannelID, "Pong! "+diff.String()+" difference from timestamp.")
	} else if cmd == "timezone" {
		if len(args) < 1 {
			var reply string
			if len(e.Mentions) > 0 {
				for _, user := range e.Mentions {
					mutexTimezones.RLock()
					timeuser, ok := timezones[user.ID]
					mutexTimezones.RUnlock()
					if ok {
						reply = user.Username + "'s timezone is " +
							timeuser.TimeZone + "."
					} else {
						reply = user.Username + "'s timezone is not set."
					}
				}
			} else {
				reply = "Usage: timezone <timezone>"
			}
			sendMessage(session, e.ChannelID, reply)
			return
		}
		is24h := len(args) >= 2 && args[1] == "24h"
		parts := strings.Split(strings.ToLower(args[0]), "/")

		for i, part := range parts {
			parts2 := strings.Split(part, "_")

			for i2, part2 := range parts2 {
				parts2[i2] = strings.Title(part2)
			}

			part = strings.Join(parts2, "_")
			parts[i] = strings.Title(part)
		}
		timezone := strings.Join(parts, "/")

		fixed, loc, err := parseTimeZone(timezone)
		if err != nil {
			sendMessage(session, e.ChannelID, "Could not load timezone.")
			return
		}

		if fixed {
			timezone = strings.ToUpper(timezone)
		}

		mutexTimezones.Lock()
		timezones[e.Author.ID] = &user{TimeZone: timezone, Is24h: is24h}
		mutexTimezones.Unlock()
		err = saveTimeZones()
		if err != nil {
			return
		}

		format := format
		if is24h {
			format = format24
		}

		currentTime := time.Now().In(loc)
		sendMessage(session, e.ChannelID, "Saved timezone \""+timezone+
			"\" for "+e.Author.Username+". Current time is "+
			currentTime.Format(format)+". "+createClockEmoji(&currentTime))
		return
	} else if cmd == "timefor" {
		mutexTimezones.RLock()
		timeuser, ok := timezones[e.Author.ID]
		mutexTimezones.RUnlock()

		format := format
		if ok && timeuser.Is24h {
			format = format24
		}

		mentions := e.Mentions
		if len(e.Mentions) <= 0 {
			mentions = []*discordgo.User{e.Author}
		}
		for _, user := range mentions {
			if nicetry(session, e.ChannelID, user) {
				return
			}

			mutexTimezones.RLock()
			timeuser, ok := timezones[user.ID]
			mutexTimezones.RUnlock()
			var reply string

			if ok {
				timezone := timeuser.TimeZone

				_, loc, err := parseTimeZone(timezone)
				if err != nil {
					stdutil.PrintErr("Invalid map entry", err)
					return
				}
				currentTime := time.Now().In(loc)
				reply = "Current time for " + user.Username + " is " +
					currentTime.Format(format) + ". " + createClockEmoji(&currentTime)
			} else {
				reply = "No timezone set for " + user.Username + "."
			}

			sendMessage(session, e.ChannelID, reply)
		}
	} else if cmd == "timeat" {
		mutexTimezones.RLock()
		timeuser, ok := timezones[e.Author.ID]
		mutexTimezones.RUnlock()
		if !ok {
			sendMessage(session, e.ChannelID, "Your timezone isn't set.")
			return
		}

		_, loc, err := parseTimeZone(timeuser.TimeZone)
		if err != nil {
			stdutil.PrintErr("Invalid map entry", err)
			return
		}

		if len(args) < 1 {
			sendMessage(session, e.ChannelID, "timeat <time> <users>")
			return
		}

		timeat := strings.ToUpper(args[0])
		t, err := time.ParseInLocation("3PM", timeat, loc)
		if err != nil {
			t, err = time.ParseInLocation("15", timeat, loc)
			if err != nil {
				sendMessage(session, e.ChannelID, "Wrong format. Example: timat 8PM @user")
				return
			}
		}
		now := time.Now().In(loc)
		t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(),
			t.Minute(), t.Second(), t.Nanosecond(), loc)

		format := format
		if timeuser.Is24h {
			format = format24
		}

		for _, user := range e.Mentions {
			if nicetry(session, e.ChannelID, user) {
				return
			}

			mutexTimezones.RLock()
			timeuser2, ok := timezones[user.ID]
			mutexTimezones.RUnlock()
			if !ok {
				sendMessage(session, e.ChannelID, user.Username+"'s "+
					"timezone isn't set.")
				return
			}

			_, loc2, err := parseTimeZone(timeuser2.TimeZone)
			if err != nil {
				stdutil.PrintErr("Invalid map entry", err)
				return
			}

			currentTime := t.In(loc2)
			sendMessage(session, e.ChannelID, timeat+" your time is "+
				currentTime.Format(format)+" for "+user.Username+". "+createClockEmoji(&currentTime))
		}
	} else if cmd == "timediff" {
		mutexTimezones.RLock()
		timeuser, ok := timezones[e.Author.ID]
		mutexTimezones.RUnlock()
		if !ok {
			sendMessage(session, e.ChannelID, "Your timezone isn't set.")
			return
		}

		_, loc, err := parseTimeZone(timeuser.TimeZone)
		if err != nil {
			stdutil.PrintErr("Invalid map entry", err)
			return
		}

		for _, user := range e.Mentions {
			if nicetry(session, e.ChannelID, user) {
				return
			}

			mutexTimezones.RLock()
			timeuser2, ok := timezones[user.ID]
			mutexTimezones.RUnlock()
			if !ok {
				sendMessage(session, e.ChannelID, user.Username+"'s "+
					"timezone isn't set.")
				return
			}

			_, loc2, err := parseTimeZone(timeuser2.TimeZone)
			if err != nil {
				stdutil.PrintErr("Invalid map entry", err)
				return
			}

			// Wanna make sure we're pretty exact.
			// update the time every time.
			now := time.Now().In(loc)
			now2 := time.Now().In(loc2)

			hour := int(now2.Hour()) - int(now.Hour())
			min := int(now2.Minute()) - int(now.Minute())
			sec := int(now2.Second()) - int(now.Second())

			hour2 := hour - 24
			min2 := min - 24
			sec2 := sec - 24

			if abs(hour2) < abs(hour) {
				hour = hour2
			}
			if abs(min2) < abs(min) {
				min = min2
			}
			if abs(sec2) < abs(sec) {
				sec = sec2
			}

			s := "Currently, the difference between you and " + user.Username + " is " +
				strconv.Itoa(int(hour)) + " hour"

			if hour != 1 {
				s += "s"
			}
			if min > 0 {
				s += " " + strconv.Itoa(min) + " minute"
				if min != 1 {
					s += "s"
				}
			}
			if sec > 0 {
				s += " " + strconv.Itoa(sec) + " minute"
				if sec != 1 {
					s += "s"
				}
			}

			sendMessage(session, e.ChannelID, s)
		}
	} else if cmd == "help" {
		dm, err := session.UserChannelCreate(e.Author.ID)
		if err != nil {
			stdutil.PrintErr("Could not open DMs", err)
			return
		}
		_, err = session.ChannelMessageSendEmbed(dm.ID,
			&discordgo.MessageEmbed{
				Color:       0x82AD,
				Title:       "TimeyWimey - Help!!!",
				Description: help,
			},
		)
		if err != nil {
			stdutil.PrintErr("Could not send embed", nil)
			return
		}
		sendMessage(session, e.ChannelID, "Delivered in DMs!")
	} else if cmd == "about" {
		_, err := session.ChannelMessageSendEmbed(e.ChannelID,
			&discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{
					Name:    "TimeyWimey",
					IconURL: avatarURL,
					URL:     discordgo.EndpointInvite(botID),
				},
				Color: 0x82AD,
				Footer: &discordgo.MessageEmbedFooter{
					Text: "Sincerely ~TimeyWimey",
				},
				Description: about,
			},
		)
		if err != nil {
			stdutil.PrintErr("Could not send embed", nil)
			return
		}
	}
}

func parseTimeZone(timezone string) (bool, *time.Location, error) {
	loc, ok := cache[timezone]
	if !ok {
		fixedPos := strings.Split(timezone, "+")
		fixedNeg := strings.Split(timezone, "-")

		if len(fixedPos) > 1 {
			zone := fixedPos[0]
			value, err := strconv.Atoi(fixedPos[1])

			if err == nil {
				loc = time.FixedZone(zone, value*60*60)
				return true, loc, nil
			}
		} else if len(fixedNeg) > 1 {
			zone := fixedNeg[0]
			value, err := strconv.Atoi(fixedNeg[1])

			if err == nil {
				loc = time.FixedZone(zone, -(value * 60 * 60))
				return true, loc, nil
			}
		}

		var err error
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return false, nil, err
		}
		cache[timezone] = loc
	}
	return false, loc, nil
}

func saveTimeZones() error {
	file, err := os.Create("timeywimey.json")
	if err != nil {
		stdutil.PrintErr("Couldn't save file", err)
		return err
	}

	mutexTimezones.RLock()
	err = json.NewEncoder(file).Encode(timezones)
	mutexTimezones.RUnlock()
	file.Close()
	if err != nil {
		stdutil.PrintErr("Could not make JSON", err)
		return err
	}
	return nil
}

func sendMessage(session *discordgo.Session, channelID, content string) {
	_, err := session.ChannelMessageSend(channelID, content)
	if err != nil {
		stdutil.PrintErr("Couldn't send message", err)
		return
	}
}
func isPermission(err error) bool {
	return strings.Contains(err.Error(), "Missing Permission")
}

func nicetry(session *discordgo.Session, channel string, user *discordgo.User) bool {
	if user.Bot {
		sendMessage(session, channel, "Nice try.")
		return true
	}
	return false
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

func createClockEmoji(t *time.Time) string {
	cHour := t.Hour()
	cMinutes := t.Minute()
	clocktext := ":clock"
	switch {
	case cMinutes >= 45:
		cMinutes = 0
		cHour++
	case cMinutes >= 20:
		cMinutes = 30
	default:
		cMinutes = 0
	}
	if cHour == 0 {
		clocktext += "12"
	} else {
		clocktext += strconv.Itoa(cHour % 12)
	}
	if cMinutes != 0 {
		clocktext += strconv.Itoa(cMinutes)
	}
	return clocktext + ":"
}
