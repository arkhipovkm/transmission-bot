package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	gtp "github.com/arkhipovkm/go-torrent-parser"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/google/uuid"
	"github.com/hekmon/transmissionrpc"
	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

var BB_SESSION string = os.Getenv("BB_SESSION")
var FORUM_URL string = os.Getenv("FORUM_URL")
var TRANSMISSION_RPC_HOST string = os.Getenv("TRANSMISSION_RPC_HOST")
var TRANSMISSION_RPC_USER string = os.Getenv("TRANSMISSION_RPC_USER")
var TRANSMISSION_RPC_PASSWORD string = os.Getenv("TRANSMISSION_RPC_PASSWORD")

type Topic struct {
	ID            string
	Verified      string
	Forum         string
	Title         string
	TitleSections []string
	Author        string
	Size          string
	Seeders       string
	Leechers      string
	Downloads     string
	CreatedAt     string
	TopicURL      string
	TorrentURL    string
	Content       *TopicContent
}

type TopicContent struct {
	ID           string
	Title        string
	ImageURL     string
	Raw          string
	Year         string
	Country      string
	Duration     string
	Genre        string
	Starring     string
	Director     string
	Description  string
	Container    string
	Subtitles    string
	Quality      string
	Translations []string
	Audios       []string
	Videos       []string
	Breadcrumb   string
}

func newHttpClient(uri string) (*http.Client, error) {
	var err error
	var client *http.Client
	_url, err := url.Parse(uri)
	if err != nil {
		return client, err
	}
	cookie := http.Cookie{
		Name:  "bb_session",
		Value: BB_SESSION,
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return client, err
	}
	jar.SetCookies(_url, []*http.Cookie{&cookie})
	client = &http.Client{
		Jar: jar,
	}
	return client, err
}

func doPOSTRequest(uri string, data url.Values) ([]byte, error) {
	var err error
	var body []byte

	client, err := newHttpClient(uri)
	if err != nil {
		return body, err
	}
	resp, err := client.PostForm(uri, data)
	if err != nil {
		return body, err
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return body, err
	}
	defer resp.Body.Close()
	body, _, err = transform.Bytes(charmap.Windows1251.NewDecoder(), body)
	if err != nil {
		return body, err
	}
	return body, err
}

func doGETRequest(uri string, query url.Values) ([]byte, error) {
	var err error
	var body []byte

	client, err := newHttpClient(uri)
	if err != nil {
		return body, err
	}
	resp, err := client.Get(uri + query.Encode())
	if err != nil {
		return body, err
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return body, err
	}
	return body, err
}

func getReplyMarkup(t string) *tgbotapi.InlineKeyboardMarkup {
	startCbData := fmt.Sprintf("start-%s", t)
	refreshCbData := fmt.Sprintf("refresh-%s", t)
	pauseCbData := fmt.Sprintf("pause-%s", t)
	removeCbData := fmt.Sprintf("remove-%s", t)
	return &tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{{
			tgbotapi.InlineKeyboardButton{
				Text:         "Start",
				CallbackData: &startCbData,
			},
			tgbotapi.InlineKeyboardButton{
				Text:         "Refresh",
				CallbackData: &refreshCbData,
			},
			tgbotapi.InlineKeyboardButton{
				Text:         "Pause",
				CallbackData: &pauseCbData,
			},
			tgbotapi.InlineKeyboardButton{
				Text:         "Remove",
				CallbackData: &removeCbData,
			},
		}},
	}
}

func cleanTextNodes(lines []string) []string {
	var newLines []string
	for _, line := range lines {
		newLine := line
		for _, char := range []string{"\t", "\n"} {
			newLine = strings.ReplaceAll(newLine, char, "")
		}
		if newLine == "" || newLine == " " {
			continue
		}
		newLines = append(newLines, newLine)
	}
	return newLines
}

func extractChildrenTextNodes(n *html.Node) []string {
	var lines []string
	if n != nil {
		if n.Type == html.TextNode {
			lines = append(lines, n.Data)
		}
		if n.Type == html.ElementNode {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				lines = append(lines, extractChildrenTextNodes(c)...)
			}
		}
	}
	return lines
}

func parseNodeText(n *html.Node) string {
	return strings.Join(cleanTextNodes(extractChildrenTextNodes(n)), " ")
}

func getTransmissionRpc() (*transmissionrpc.Client, error) {
	return transmissionrpc.New(TRANSMISSION_RPC_HOST, "", "", nil)
}

func getTorrentFile(t string) (string, []byte, error) {
	var fileName string
	var body []byte
	var err error

	fileName = filepath.Join("torrents", fmt.Sprintf("%s.torrent", t))
	body, err = ioutil.ReadFile(fileName)
	if err != nil {
		log.Printf("Could not find torrent %s in saved torrents. Downloading from the forum..", t)
		body, err = doGETRequest(FORUM_URL+"/dl.php", url.Values{"t": []string{t}})
		if err != nil {
			return fileName, body, err
		}
		_ = ioutil.WriteFile(fileName, body, os.ModePerm)
	}
	if len(body) == 0 {
		return fileName, body, err
	}
	return fileName, body, err

}

func getTransmissionTorrentInfo(tm *transmissionrpc.Client, hash string) (string, string, float64, error) {
	var err error
	var torrentName string
	var torrentStatus transmissionrpc.TorrentStatus
	var torrentPercent float64
	torrents, err := tm.TorrentGetHashes(
		[]string{"id", "name", "status", "percentDone"},
		[]string{hash},
	)
	if err != nil {
		return torrentName, torrentStatus.String(), torrentPercent, err
	}
	if len(torrents) == 0 {
		return torrentName, torrentStatus.String(), torrentPercent, err
	}
	torrent := torrents[0]
	if torrent.Name != nil {
		torrentName = *torrent.Name
	}
	if torrent.Status != nil {
		torrentStatus = *torrent.Status
	}
	if torrent.PercentDone != nil {
		torrentPercent = *torrent.PercentDone
	}
	return torrentName, torrentStatus.String(), torrentPercent, err
}

func getUpdatedTorrentInfoMessage(tm *transmissionrpc.Client, t string) (*tgbotapi.EditMessageTextConfig, error) {
	var err error
	_, body, err := getTorrentFile(t)
	if err != nil {
		return nil, err
	}
	torrentFile, err := gtp.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	torrentName, torrentStatus, torrentPercent, err := getTransmissionTorrentInfo(tm, torrentFile.InfoHash)
	msg := tgbotapi.NewEditMessageText(
		0,
		0,
		fmt.Sprintf("%s: %s (%.1f%%)", torrentName, torrentStatus, torrentPercent*100),
	)
	msg.ReplyMarkup = getReplyMarkup(t)
	return &msg, err
}

func getTopics(query string) ([]*Topic, error) {
	var topics []*Topic
	var err error

	form := url.Values{
		"nm": {query},
		"o":  {"7"},
		"s":  {"2"},
	}
	body, err := doPOSTRequest(FORUM_URL+"/tracker.php", form)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return topics, err
	}

	var currentTopic *Topic

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && strings.Contains(attr.Val, "trs-tr-") {
					_id := strings.Split(attr.Val, "trs-tr-")
					currentTopic = &Topic{
						ID: _id[1],
					}
					topics = append(topics, currentTopic)
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "td" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "f-name-col") {
					currentTopic.Forum = parseNodeText(n)
				}
				if attr.Key == "class" && strings.Contains(attr.Val, "t-title-col") {
					currentTopic.TitleSections = cleanTextNodes(extractChildrenTextNodes(n))
					currentTopic.Title = parseNodeText(n)
				}
				if attr.Key == "class" && strings.Contains(attr.Val, "u-name-col") {
					currentTopic.Author = parseNodeText(n)
				}
				if attr.Key == "class" && strings.Contains(attr.Val, "tor-size") {
					currentTopic.Size = parseNodeText(n)
					currentTopic.Size = strings.ReplaceAll(currentTopic.Size, " ↓", "")
				}
				if attr.Key == "class" && strings.Contains(attr.Val, "row4 leechmed bold") {
					currentTopic.Leechers = parseNodeText(n)
				}
				if attr.Key == "class" && strings.Contains(attr.Val, "row4 small number-format") {
					currentTopic.Downloads = parseNodeText(n)
				}
				if attr.Key == "data-ts_text" {
					currentTopic.CreatedAt = parseNodeText(n)
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "b" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "seedmed") {
					currentTopic.Seeders = n.FirstChild.Data
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "span" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "tor-icon tor-") {
					currentTopic.Verified = n.FirstChild.Data
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return topics, err
}

func getSectionInlineResults(query string, offset int) (results []interface{}, nextOffset string, err error) {
	nextOffset = strconv.Itoa(offset + 50)
	topics, err := getTopics(query)
	if err != nil {
		return results, nextOffset, err
	}
	for _, topic := range topics {

		var description string = topic.Size
		if topic.Seeders != "" {
			description += " : " + topic.Seeders
		}
		if topic.Verified != "" {
			description += " : " + topic.Verified
		}

		text := fmt.Sprintf("<b>%s</b>\n", topic.Title) + fmt.Sprintf("Size: %s\nSeeders: %s\nDownloads: %s", topic.Size, topic.Seeders, topic.Downloads) + "\n" //+ topic.Content.Breadcrumb + coverSuffix
		inputMessageContent := &tgbotapi.InputTextMessageContent{
			Text:                  text,
			ParseMode:             "HTML",
			DisableWebPagePreview: false,
		}

		downloadCbData := fmt.Sprintf("init-%s", topic.ID)
		topicURL := FORUM_URL + "?t=" + topic.ID
		results = append(results, &tgbotapi.InlineQueryResultArticle{
			Type:                "article",
			ID:                  uuid.New().String(),
			Title:               topic.Title,
			Description:         description,
			InputMessageContent: inputMessageContent,
			HideURL:             true,
			ReplyMarkup: &tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{{
					tgbotapi.InlineKeyboardButton{
						Text:         "Download",
						CallbackData: &downloadCbData,
					},
					tgbotapi.InlineKeyboardButton{
						Text: "View topic",
						URL:  &topicURL,
					},
					tgbotapi.InlineKeyboardButton{
						Text:                         "Back",
						SwitchInlineQueryCurrentChat: &query,
					},
				}},
			},
		})
	}
	log.Printf("Got %d results from tracker..\n", len(results))
	return results, nextOffset, err
}

func process(bot *tgbotapi.BotAPI, updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		if update.Message != nil && update.Message.Text != "" {
			uri, err := url.ParseRequestURI(update.Message.Text)
			if err != nil {
				log.Println(err)
				continue
			}
			query := uri.Query()
			t := query.Get("t")
			if t == "" {
				continue
			}
			_, body, err := getTorrentFile(t)
			if err != nil {
				log.Println(err)
				continue
			}
			torrentFile, err := gtp.Parse(bytes.NewReader(body))
			if err != nil {
				log.Println(err)
			}
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(
				"%s: ready to start", torrentFile.Info.Name,
			))
			msg.ReplyToMessageID = update.Message.MessageID
			startCbData := fmt.Sprintf("start-%s", t)
			msg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{{
					tgbotapi.InlineKeyboardButton{
						Text:         "Start",
						CallbackData: &startCbData,
					},
				}},
			}
			bot.Send(msg)
		} else if update.CallbackQuery != nil {
			var chatID int64
			if update.CallbackQuery.Message != nil &&
				update.CallbackQuery.Message.Chat != nil &&
				update.CallbackQuery.Message.Chat.ID != 0 {
				chatID = update.CallbackQuery.Message.Chat.ID
			} else if update.CallbackQuery.From != nil {
				chatID = int64(update.CallbackQuery.From.ID)
			} else {
				continue
			}
			var re *regexp.Regexp
			re = regexp.MustCompile("^start-(.*?)$")
			if re.MatchString(update.CallbackQuery.Data) {
				parts := re.FindStringSubmatch(update.CallbackQuery.Data)
				if len(parts) < 2 {
					log.Println("Invalid callback query regexp match.")
				}
				t := parts[1]
				tm, err := getTransmissionRpc()
				if err != nil {
					log.Println(err)
					continue
				}
				fileName, _, err := getTorrentFile(t)
				if err != nil {
					log.Println(err)
					continue
				}
				torrent, err := tm.TorrentAddFile(fileName)
				if err != nil {
					log.Println(err)
					continue
				}
				tm.TorrentStartIDs([]int64{*torrent.ID})
				_, err = bot.AnswerCallbackQuery(
					tgbotapi.NewCallback(
						update.CallbackQuery.ID,
						fmt.Sprintf("Started torrent: %s", *torrent.Name),
					),
				)
				if err != nil {
					log.Println(err)
				}

				msg, err := getUpdatedTorrentInfoMessage(tm, t)
				if err != nil {
					log.Println(err)
					continue
				}
				msg.ChatID = chatID
				msg.MessageID = update.CallbackQuery.Message.MessageID
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
					continue
				}
			}
			re = regexp.MustCompile("^pause-(.*?)$")
			if re.MatchString(update.CallbackQuery.Data) {
				parts := re.FindStringSubmatch(update.CallbackQuery.Data)
				if len(parts) < 2 {
					log.Println("Invalid callback query regexp match.")
				}
				t := parts[1]
				tm, err := getTransmissionRpc()
				if err != nil {
					log.Println(err)
					continue
				}
				_, body, err := getTorrentFile(t)
				if err != nil {
					log.Println(err)
					continue
				}
				torrentFile, err := gtp.Parse(bytes.NewReader(body))
				if err != nil {
					log.Println(err)
					continue
				}
				err = tm.TorrentStopHashes([]string{torrentFile.InfoHash})
				if err != nil {
					log.Println(err)
					continue
				}
				_, err = bot.AnswerCallbackQuery(
					tgbotapi.NewCallback(
						update.CallbackQuery.ID,
						fmt.Sprintf("Stopped torrent: %s", torrentFile.Info.Name),
					),
				)
				if err != nil {
					log.Println(err)
				}

				msg, err := getUpdatedTorrentInfoMessage(tm, t)
				if err != nil {
					log.Println(err)
					continue
				}
				msg.ChatID = chatID
				msg.MessageID = update.CallbackQuery.Message.MessageID
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
					continue
				}
			}
			re = regexp.MustCompile("^refresh-(.*?)$")
			if re.MatchString(update.CallbackQuery.Data) {
				parts := re.FindStringSubmatch(update.CallbackQuery.Data)
				if len(parts) < 2 {
					log.Println("Invalid callback query regexp match.")
				}
				t := parts[1]
				tm, err := getTransmissionRpc()
				if err != nil {
					log.Println(err)
					continue
				}
				msg, err := getUpdatedTorrentInfoMessage(tm, t)
				if err != nil {
					log.Println(err)
					continue
				}
				msg.ChatID = chatID
				msg.MessageID = update.CallbackQuery.Message.MessageID
				bot.Send(msg)

				_, err = bot.AnswerCallbackQuery(
					tgbotapi.NewCallback(
						update.CallbackQuery.ID,
						"",
					),
				)
				if err != nil {
					log.Println(err)
					continue
				}
			}
			re = regexp.MustCompile("^remove-yes-(.*?)$")
			if re.MatchString(update.CallbackQuery.Data) {
				parts := re.FindStringSubmatch(update.CallbackQuery.Data)
				if len(parts) < 2 {
					log.Println("Invalid callback query regexp match.")
				}
				t := parts[1]
				tm, err := getTransmissionRpc()
				if err != nil {
					log.Println(err)
					continue
				}
				_, body, err := getTorrentFile(t)
				if err != nil {
					log.Println(err)
					continue
				}
				torrentFile, err := gtp.Parse(bytes.NewReader(body))
				if err != nil {
					log.Println(err)
					continue
				}
				torrents, err := tm.TorrentGetHashes(
					[]string{"id"},
					[]string{torrentFile.InfoHash},
				)
				if err != nil {
					log.Println(err)
					continue
				}
				if len(torrents) < 1 {
					continue
				}
				torrent := torrents[0]
				err = tm.TorrentRemove(&transmissionrpc.TorrentRemovePayload{
					IDs:             []int64{*torrent.ID},
					DeleteLocalData: true,
				})
				if err != nil {
					log.Println(err)
					continue
				}

				_, err = bot.AnswerCallbackQuery(
					tgbotapi.NewCallback(
						update.CallbackQuery.ID,
						fmt.Sprintf("Removed torrent: %s", torrentFile.Info.Name),
					),
				)
				if err != nil {
					log.Println(err)
					continue
				}
				msg := tgbotapi.NewEditMessageText(
					chatID,
					update.CallbackQuery.Message.MessageID,
					fmt.Sprintf("%s: removed", torrentFile.Info.Name),
				)
				startCbData := fmt.Sprintf("start-%s", t)
				msg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
					InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{{
						tgbotapi.InlineKeyboardButton{
							Text:         "Restart",
							CallbackData: &startCbData,
						},
					}},
				}
				_, err = bot.Send(&msg)
				if err != nil {
					log.Println(err)
					continue
				}
				continue
			}
			re = regexp.MustCompile("^remove-(.*?)$")
			if re.MatchString(update.CallbackQuery.Data) {
				parts := re.FindStringSubmatch(update.CallbackQuery.Data)
				if len(parts) < 2 {
					log.Println("Invalid callback query regexp match.")
				}
				t := parts[1]
				_, body, err := getTorrentFile(t)
				if err != nil {
					log.Println(err)
					continue
				}
				torrentFile, err := gtp.Parse(bytes.NewReader(body))
				if err != nil {
					log.Println(err)
					continue
				}
				msg := tgbotapi.NewEditMessageText(
					chatID,
					update.CallbackQuery.Message.MessageID,
					fmt.Sprintf("Are you sure you want to remove torrent \"%s\" and all its contents?", torrentFile.Info.Name),
				)
				removeYesCbData := fmt.Sprintf("remove-yes-%s", t)
				removeNoCbData := fmt.Sprintf("refresh-%s", t)
				msg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
					InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{{
						tgbotapi.InlineKeyboardButton{
							Text:         "Yes",
							CallbackData: &removeYesCbData,
						},
						tgbotapi.InlineKeyboardButton{
							Text:         "No",
							CallbackData: &removeNoCbData,
						},
					}},
				}
				if err != nil {
					log.Println(err)
					continue
				}
				bot.Send(msg)

				_, err = bot.AnswerCallbackQuery(
					tgbotapi.NewCallback(
						update.CallbackQuery.ID,
						"",
					),
				)
				if err != nil {
					log.Println(err)
					continue
				}
			}
			re = regexp.MustCompile("^init-(.*?)$")
			if re.MatchString(update.CallbackQuery.Data) {
				parts := re.FindStringSubmatch(update.CallbackQuery.Data)
				if len(parts) < 2 {
					log.Println("Invalid callback query regexp match.")
				}
				t := parts[1]
				tm, err := getTransmissionRpc()
				if err != nil {
					log.Println(err)
					continue
				}
				fileName, _, err := getTorrentFile(t)
				if err != nil {
					log.Println(err)
					continue
				}
				torrent, err := tm.TorrentAddFile(fileName)
				if err != nil {
					log.Println(err)
					continue
				}
				tm.TorrentStartIDs([]int64{*torrent.ID})

				torrentName, torrentStatus, torrentPercent, err := getTransmissionTorrentInfo(tm, *torrent.HashString)
				if err != nil {
					log.Println(err)
					continue
				}
				msg := tgbotapi.NewMessage(
					chatID,
					fmt.Sprintf("%s: %s (%.1f%%)", torrentName, torrentStatus, torrentPercent*100),
				)
				msg.ChatID = chatID
				msg.ReplyMarkup = getReplyMarkup(t)

				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				_, err = bot.AnswerCallbackQuery(
					tgbotapi.NewCallback(
						update.CallbackQuery.ID,
						fmt.Sprintf("Started downloading torrent: %s", *torrent.Name),
					),
				)
				if err != nil {
					log.Println(err)
					continue
				}
			}
		} else if update.InlineQuery != nil {
			log.Println("Got an inline query", update.InlineQuery.Query)
			inlineQueryAnswer := tgbotapi.InlineConfig{
				InlineQueryID: update.InlineQuery.ID,
				CacheTime:     0,
				IsPersonal:    false,
			}
			if update.InlineQuery.Query == "" || update.InlineQuery.Query == " " {
				inlineQueryAnswer.CacheTime = 0
				_, err := bot.AnswerInlineQuery(inlineQueryAnswer)
				if err != nil {
					log.Println(err)
					break
				}
			} else {
				var offset int
				var err error
				if update.InlineQuery.Offset != "" {
					offset, _ = strconv.Atoi(update.InlineQuery.Offset)
				}
				inlineQueryAnswer.Results, _, err = getSectionInlineResults(update.InlineQuery.Query, offset)
				if err != nil {
					log.Println(err)
				}
				log.Println("Sending Inline Answer..")
				bot.AnswerInlineQuery(inlineQueryAnswer)
			}
		}
	}
}

func main() {
	telegramBotApiToken := os.Getenv("BOT_TOKEN")
	if telegramBotApiToken == "" {
		panic("No Bot API Token provided")
	}
	FORUM_URL = os.Getenv("FORUM_URL")
	if FORUM_URL == "" {
		panic("No Forum URL provided")
	}
	BB_SESSION = os.Getenv("BB_SESSION")
	if BB_SESSION == "" {
		panic("No BB SESSION cookie provided")
	}
	TRANSMISSION_RPC_HOST = os.Getenv("TRANSMISSION_RPC_HOST")
	if TRANSMISSION_RPC_HOST == "" {
		panic("No TRANSMISSION_RPC_HOST provided")
	}
	TRANSMISSION_RPC_USER = os.Getenv("TRANSMISSION_RPC_USER")
	TRANSMISSION_RPC_PASSWORD = os.Getenv("TRANSMISSION_RPC_PASSWORD")

	APP_HOSTNAME := os.Getenv("APP_HOSTNAME")
	if APP_HOSTNAME == "" {
		panic("No APP_HOSTMANE provided")
	}

	os.Mkdir("torrents", os.ModePerm)

	bot, err := tgbotapi.NewBotAPI(telegramBotApiToken)
	if err != nil {
		log.Panic(err)
	}

	debug := false
	debugEnv := os.Getenv("DEBUG")
	if debugEnv != "" {
		debug = true
	}
	bot.Debug = debug
	log.Printf("Authorized on account %s", bot.Self.UserName)

	var updates tgbotapi.UpdatesChannel
	if !debug {
		_, err = bot.SetWebhook(tgbotapi.NewWebhook(fmt.Sprintf("https://%s/%s", APP_HOSTNAME, bot.Token)))
		if err != nil {
			log.Fatal(err)
		}
		info, err := bot.GetWebhookInfo()
		if err != nil {
			log.Fatal(err)
		}
		if info.LastErrorDate != 0 {
			log.Printf("Telegram callback failed: %s", info.LastErrorMessage)
		}
		updates = bot.ListenForWebhook("/" + bot.Token)
	} else {
		_, err = bot.RemoveWebhook()
		if err != nil {
			panic(err)
		}
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates, err = bot.GetUpdatesChan(u)
		if err != nil {
			panic(err)
		}
	}

	for w := 0; w < runtime.NumCPU()+2; w++ {
		go process(bot, updates)
	}
}
