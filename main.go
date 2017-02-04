package main;

import (
	"fmt"
	"os"
	"io/ioutil"
	"encoding/json"
	"github.com/bwmarrin/discordgo"
	"strings"
	"time"
)

const PREFIX = "#";
const BOTID = "277346768222420992";
const FORMAT = "03:04 PM";
const FORMAT24 = "15:04";
const HELP = `
**Welcome to TimeyWimey!**
This is the bot that manages your timezones... for you.

Specify your timezone:
` + "```#timezone <timezone> [24h]```" + `
Get time of user:
` + "```#timefor @user1 @user2 et.c```" + `
Get time of user at specific time (BETA):
` + "```#timeat <time> @users```" + `
Examples:
` + "```#timezone europe/stockholm```" + `
Saved timezone "Europe/Stockholm" for LEGOlord208. Current time is 06:66 AM
` + "```#timefor```" + `
Current time for LEGOlord208 is 06:66 AM.
` + "```#timefor @test @LEGOlord208```" + `
Current time for test is 07:66 AM.
Current time for LEGOlord208 is 06:66 AM.
` + "```#timeat 7PM @LEGOlord208```" + `
7PM your time is 08:00 PM for test.
`

type User struct{
	TimeZone string
	Is24h bool `json:",omitempty"`
}

var timezones map[string]*User;
var cache = make(map[string]*time.Location);

func main(){
	args := os.Args[1:];
	
	if(len(args) < 1){
		fmt.Println("No token supplied in arguments.");
		return;
	}
	token := args[0];

	fmt.Println("Loading...");

	data, err := ioutil.ReadFile("timeywimey.json");
	if(err != nil){
		fmt.Println("Note: Using empty timeywimey.json");
		timezones = make(map[string]*User);
	} else {
		err = json.Unmarshal(data, &timezones);
		if(err != nil){
			printErr("Could not load JSON", err);
			return;
		}
	}

	fmt.Println("Starting...");

	session, err := discordgo.New("Bot " + token);
	if(err != nil){
		printErr("Could not create Discord session", err);
		return;
	}

	session.AddHandler(messageCreate);
	session.AddHandler(messageUpdate);

	err = session.Open();
	if(err != nil){
		printErr("Could not start Discord session", err);
		return;
	}

	fmt.Println("Started!");

	<-make(chan interface{});
}

func messageCreate(session *discordgo.Session, e *discordgo.MessageCreate){
	message(session, e.Message);
}
func messageUpdate(session *discordgo.Session, e *discordgo.MessageUpdate){
	message(session, e.Message);
}
func message(session *discordgo.Session, e *discordgo.Message){
	if(e.Author == nil){ return; }
	if(e.Author.Bot){ return; }
	msg := strings.ToLower(strings.TrimSpace(e.Content));

	if(msg == ""){
		return;
	}
	if(!strings.HasPrefix(msg, PREFIX)){
		return;
	}
	msg = msg[1:];

	parts := strings.Split(msg, " ");
	cmd := parts[0];
	args := parts[1:];

	if(cmd == "timezone"){
		if(len(args) < 1){
			sendMessage(session, e.ChannelID, "Usage: " + PREFIX +
				"timezone <timezone>");
			return;
		}
		is24h := len(args) >= 2 && args[1] == "24h";
		parts := strings.Split(strings.ToLower(args[0]), "/");

		for i, part := range parts{
			parts2 := strings.Split(part, "_");
			
			for i2, part2 := range parts2{
				parts2[i2] = strings.Title(part2);
			}
			
			part = strings.Join(parts2, "_");
			parts[i] = strings.Title(part);
		}
		timezone := strings.Join(parts, "/");

		loc, err := parseTimeZone(timezone);
		if(err != nil){
			sendMessage(session, e.ChannelID, "Could not load timezone.");
			return;
		}

		timezones[e.Author.ID] = &User{TimeZone: timezone, Is24h: is24h};
		err = saveTimeZones();
		if(err != nil){
			return;
		}
		
		format := FORMAT;
		if(is24h){
			format = FORMAT24;
		}

		currentTime := time.Now().In(loc).Format(format);
		sendMessage(session, e.ChannelID, "Saved timezone \"" + timezone +
			"\" for " + e.Author.Username + ". Current time is " +
			currentTime);
		return;
	} else if(cmd == "timefor"){
		timeuser, ok := timezones[e.Author.ID];

		format := FORMAT;
		if(ok && timeuser.Is24h){
			format = FORMAT24;
		}

		mentions := e.Mentions;
		if(len(e.Mentions) <= 0){
			mentions = []*discordgo.User{e.Author};
		}
		for _, user := range mentions{
			if(user.ID == BOTID){
				sendMessage(session, e.ChannelID, "Nice try.");
				return;
			}

			timeuser, ok := timezones[user.ID];
			var reply string;
			
			if(ok){
				timezone := timeuser.TimeZone;

				loc, err := parseTimeZone(timezone);
				if(err != nil){
					printErr("Invalid map entry", err);
					return;
				}
				currentTime := time.Now().In(loc).Format(format);

				reply = "Current time for " + user.Username + " is " +
					currentTime + ".";
			} else {
				reply = "No timezone set for " + user.Username + ".";
			}

			sendMessage(session, e.ChannelID, reply);
		}
	} else if(cmd == "timeat"){
		timeuser, ok := timezones[e.Author.ID];
		if(!ok){
			sendMessage(session, e.ChannelID, "Your timezone isn't set.");
			return;
		}

		loc, err := parseTimeZone(timeuser.TimeZone);
		if(err != nil){
			printErr("Invalid map entry", err);
			return;
		}

		if(len(args) < 1){
			sendMessage(session, e.ChannelID, PREFIX +
				"timeat <time> <users>");
			return;
		}

		timeat := strings.ToUpper(args[0]);
		t, err := time.ParseInLocation("3PM", timeat, loc);
		if(err != nil){
			t, err = time.ParseInLocation("15", timeat, loc);
			if(err != nil){
				sendMessage(session, e.ChannelID, "Wrong format. Example: " +
					PREFIX + "timat 8PM @user");
				return;
			}
		}

		format := FORMAT;
		if(timeuser.Is24h){
			format = FORMAT24;
		}

		for _, user := range e.Mentions{
			if(user.ID == BOTID){
				sendMessage(session, e.ChannelID, "Nice try.");
				return;
			}
			
			timeuser2, ok := timezones[user.ID];
			if(!ok){
				sendMessage(session, e.ChannelID, user.Username + "'s " +
					"timezone isn't set.");
				return;
			}

			loc2, err := parseTimeZone(timeuser2.TimeZone);
			if(err != nil){
				printErr("Invalid map entry", err);
				return;
			}

			currentTime := t.In(loc2).Format(format);
			sendMessage(session, e.ChannelID, timeat + " your time is " +
				currentTime + " for " + user.Username + ".");
		}
	} else if(cmd == "help"){
		dm, err := session.UserChannelCreate(e.Author.ID);
		if(err != nil){
			printErr("Could not open DMs", err);
			return;
		}
		sendMessage(session, dm.ID, HELP);
		sendMessage(session, e.ChannelID, "Delivered in DMs!");
	}
}

func parseTimeZone(timezone string) (*time.Location, error){
	loc, ok := cache[timezone];
	if(!ok){
		var err error;
		loc, err = time.LoadLocation(timezone);
		if(err != nil){
			return nil, err;
		}
		cache[timezone] = loc;
	}
	return loc, nil;
}

func saveTimeZones() error{
	data, err := json.Marshal(timezones);
	if(err != nil){
		printErr("Could not make JSON", err);
		return err;
	}

	err = ioutil.WriteFile("timeywimey.json", data, 0666);
	if(err != nil){
		printErr("Couldn't save file", err);
		return err;
	}
	return nil;
}

func sendMessage(session *discordgo.Session, channelID, content string){
	_, err := session.ChannelMessageSend(channelID, content);
	if(err != nil){
		printErr("Couldn't send message", err);
		return;
	}
}

func printErr(prefix string, err error){
	fmt.Fprintln(os.Stderr, prefix + ":", err);
}
