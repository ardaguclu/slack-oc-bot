package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ardaguclu/slack-oc-bot/filemanager"
)

func main() {
	godotenv.Load(".env")
	token := os.Getenv("SLACK_AUTH_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")

	client := slack.New(token, slack.OptionDebug(false), slack.OptionAppLevelToken(appToken))

	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	fm := filemanager.NewFileManager()

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
					HandleEventMessage(eventsAPI, client, socketClient, fm)
				}
			}
		}
	}(ctx, client, socketClient)

	socketClient.Run()
}

func HandleEventMessage(event slackevents.EventsAPIEvent, client *slack.Client, socketClient *socketmode.Client, fm *filemanager.FileManager) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			callback, ok := event.Data.(*slackevents.EventsAPICallbackEvent)
			var m *slackevents.MessageEvent
			if ok {
				d, err := callback.InnerEvent.MarshalJSON()
				if err != nil {
					return
				}
				err = json.Unmarshal(d, &m)
				if err != nil {
					return
				}
			}

			output, err := HandleAppMentionEventToBot(ev, client, socketClient, fm, m)
			if err != nil {
				ts := ev.ThreadTimeStamp
				if ev.ThreadTimeStamp == "" {
					ts = ev.TimeStamp
				}
				_, _, _, err = socketClient.SendMessage(ev.Channel, slack.MsgOptionTS(ts), slack.MsgOptionText(err.Error(), false))
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
				_, _, _, err = socketClient.SendMessage(ev.Channel, slack.MsgOptionTS(ts), slack.MsgOptionText(output, false))
				if err != nil {
					log.Printf("error %v", err)
				}
			}
		}
	}
}

func HandleAppMentionEventToBot(event *slackevents.AppMentionEvent, client *slack.Client, socketClient *socketmode.Client, fm *filemanager.FileManager, m *slackevents.MessageEvent) (string, error) {
	rgxUpload, _ := regexp.Compile("<@[\\w\\d]+>\\s*upload")
	rgxOC, _ := regexp.Compile("<@[\\w\\d]+>\\s*(kubectl|oc) ")
	if rgxUpload.MatchString(event.Text) {
		var kubeconfigURL string
		if m == nil || len(m.Files) == 0 {
			user, err := client.GetUserInfo(event.User)
			if err != nil {
				return "", err
			}

			files, _, err := socketClient.GetFiles(slack.GetFilesParameters{
				User:    user.ID,
				Channel: event.Channel,
				Types:   "snippets",
			})
			if err != nil || len(files) == 0 {
				return "", fmt.Errorf("please import valid kubeconfig file or code snippet")
			}

			kubeconfigURL = files[0].URLPrivateDownload
		} else {
			kubeconfigURL = m.Files[0].URLPrivateDownload
		}
		buffer := &bytes.Buffer{}
		err := client.GetFile(kubeconfigURL, buffer)
		if err != nil {
			return "", err
		}

		_, err = clientcmd.Load([]byte(buffer.String()))
		if err != nil {
			return "", err
		}

		err = fm.Add(event.Channel, buffer.Bytes())
		if err != nil {
			return "", err
		}

		return "config file is successfully uploaded", nil
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
			return fmt.Sprintf("%s\n```\n%s\n", err, string(output)), nil
		}
		return fmt.Sprintf("```\n%s\n```\n", string(output)), nil
	}

	return "", fmt.Errorf("invalid command")
}
