package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	godotenv.Load(".env")
	token := os.Getenv("SLACK_AUTH_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")

	client := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))

	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	fm := NewFileManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func(ctx context.Context, client *slack.Client, socketClient *socketmode.Client) {
		for {
			select {
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socketClient.Events:

				switch event.Type {

				case socketmode.EventTypeEventsAPI:

					eventsAPI, ok := event.Data.(slackevents.EventsAPIEvent)
					if !ok {
						log.Printf("Could not type cast the event to the EventsAPI: %v\n", event)
						continue
					}
					socketClient.Ack(*event.Request)
					HandleEventMessage(eventsAPI, client, fm)
				}
			}
		}
	}(ctx, client, socketClient)

	socketClient.Run()
}

func HandleEventMessage(event slackevents.EventsAPIEvent, client *slack.Client, fm *FileManager) {
	switch event.Type {
	case slackevents.CallbackEvent:

		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			output, err := HandleAppMentionEventToBot(ev, client, fm)
			if err != nil {
				ts := ev.ThreadTimeStamp
				if ev.ThreadTimeStamp == "" {
					ts = ev.TimeStamp
				}
				_, _, _, err = client.SendMessage(ev.Channel, slack.MsgOptionTS(ts), slack.MsgOptionText(err.Error(), false))
				if err != nil {
					log.Printf("error %v", err)
				}
				return
			}
			if output != "" {
				ts := ev.ThreadTimeStamp
				if ev.ThreadTimeStamp == "" {
					ts = ev.TimeStamp
				}
				_, _, _, err = client.SendMessage(ev.Channel, slack.MsgOptionTS(ts), slack.MsgOptionText(output, false))
				if err != nil {
					log.Printf("error %v", err)
				}
			}
		}
	}
}

func HandleAppMentionEventToBot(event *slackevents.AppMentionEvent, client *slack.Client, fm *FileManager) (string, error) {
	user, err := client.GetUserInfo(event.User)
	if err != nil {
		return "", err
	}

	rgxUpload, _ := regexp.Compile("<@[\\w\\d]+>\\s*upload")
	rgxOC, _ := regexp.Compile("<@[\\w\\d]+>\\s*(kubectl|oc)")
	if rgxUpload.MatchString(event.Text) {
		var ts *slack.JSONTime
		err = ts.UnmarshalJSON([]byte(event.EventTimeStamp))
		if err != nil {
			return "", err
		}

		files, _, err := client.GetFiles(slack.GetFilesParameters{
			User:          user.ID,
			Channel:       event.Channel,
			TimestampFrom: *ts,
			Types:         "snippet",
			Count:         1,
		})
		if err != nil || len(files) == 0 {
			return "", fmt.Errorf("please import valid kubeconfig file or code snippet")
		}

		kubeconfig := files[0]
		fmt.Println(kubeconfig)

		return "", nil
	} else if rgxOC.MatchString(event.Text) {
		path, err := fm.Get(event.Channel)
		if err != nil {
			return "", err
		}

		parsed := strings.Split(rgxOC.ReplaceAllString(event.Text, ""), " ")
		parsed = append(parsed, fmt.Sprintf("--kubeconfig=%s", path))

		cmd := exec.Command("oc", parsed...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("```\n%s\n```\n", string(output)), nil
	}

	return "", fmt.Errorf("invalid command")
}
