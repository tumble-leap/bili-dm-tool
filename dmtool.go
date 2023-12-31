package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/AceXiamo/blivedm-go/api"
	"github.com/AceXiamo/blivedm-go/client"
	"github.com/AceXiamo/blivedm-go/message"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

func NewBiliDmTool(configFile, cookieFile string, logLevel log.Level) *BiliDmTool {
	return &BiliDmTool{
		ConfigFile: configFile,
		CookieFile: cookieFile,
		LogLevel:   logLevel,
	}
}

func (tool *BiliDmTool) run() error {
	log.Printf("当前管理员uid为：%d，呼叫 %s 可检测是否离线！", tool.Admin, tool.Nick)
	count := 0
	for i, room := range tool.Rooms {
		if room.Id == 0 || !room.Enable {
			continue
		}
		count++
		room := room
		i := i
		go func() {
			uname, err := getUnameByRoom(room.Id)
			if err != nil {
				log.Println(err)
				uname = "unknown"
			}
			log.Printf("正在连接第%d个直播间，房间id：%d，主播：%s", i+1, room.Id, uname)
			c := client.NewClient(room.Id)
			c.SetCookie(tool.cookies)

			//弹幕事件
			c.OnDanmaku(func(danmaku *message.Danmaku) {
				if danmaku.Type == message.EmoticonDanmaku {
					log.Printf("[弹幕表情] %s：表情URL： %s\n", danmaku.Sender.Uname, danmaku.Emoticon.Url)
				} else {
					log.Printf("[弹幕] %s：%s\n", danmaku.Sender.Uname, danmaku.Content)
					if danmaku.Sender.Uid == tool.Admin && strings.Contains(danmaku.Content, tool.Nick) {
						tool.sendDanmaku(c.RoomID, "在的呢～")
					}
				}
			})

			// 醒目留言事件
			c.OnSuperChat(func(superChat *message.SuperChat) {
				log.Printf("[SC|%d元] %s: %s\n", superChat.Price, superChat.UserInfo.Uname, superChat.Message)
			})

			// 礼物事件
			if room.ThankGift {
				c.OnGift(func(gift *message.Gift) {
					if gift.CoinType == "gold" {
						log.Printf("[礼物] %s 的 %s %d 个 共%.2f元\n", gift.Uname, gift.GiftName, gift.Num, float64(gift.Num*gift.Price)/1000)
						if len([]rune(gift.GiftName)) > 6 {
							gift.GiftName = string([]rune(gift.GiftName)[0:6])
						}
						tool.sendDanmaku(c.RoomID, strTrans("感谢%s的"+gift.GiftName+","+room.ThankGiftMessage[rand.Intn(len(room.ThankGiftMessage))], gift.Uname))
					}
				})
				log.Println("礼物感谢事件注册成功")
			}

			// 上舰事件
			if room.ThankGuard {
				c.OnGuardBuy(func(guardBuy *message.GuardBuy) {
					log.Printf("[大航海] %s 开通了 %d 等级的大航海，金额 %d 元\n", guardBuy.Username, guardBuy.GuardLevel, guardBuy.Price/1000)
					tool.sendDanmaku(c.RoomID, strTrans("感谢%s上船,"+room.ThankGuardMessage[rand.Intn(len(room.ThankGuardMessage))], guardBuy.Username))

				})
				log.Println("舰长感谢事件注册成功")
			}

			// 监听进入直播间事件
			if room.AutoWelcome {
				c.RegisterCustomEventHandler("INTERACT_WORD", func(s string) {
					var v message.InteractWord
					data := gjson.Get(s, "data").String()
					json.Unmarshal([]byte(data), &v)
					tool.sendDanmaku(c.RoomID, strTrans(room.WelcomeMessage, v.Uname))
				})
				log.Println("进入直播间欢迎事件注册成功")
			}

			err = c.Start()
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("第%d个直播间连接成功", i+1)

			if room.AutoSend {
				log.Println("自动轮发消息事件注册成功")
				time.Sleep(time.Minute)
				for {
					for _, m := range room.Messages {
						tool.sendDanmaku(room.Id, m)
						time.Sleep(time.Second * time.Duration(rand.Intn(tool.MaxDura-tool.MinDura)+tool.MinDura))
					}
				}
			} else {
				select {}
			}
		}()
	}
	if count != 0 {
		select {}
	} else {
		return errors.New("所有直播间都未启用")
	}
}

func (tool *BiliDmTool) sendDanmaku(roomid int, msg string) error {
	if tool.biliJct == "" || tool.sessData == "" {
		return nil
	}
	dmReq := &api.DanmakuRequest{
		Msg:      msg,
		RoomID:   fmt.Sprint(roomid),
		Bubble:   "0",
		Color:    "16777215",
		FontSize: "25",
		Mode:     "1",
		DmType:   "0",
	}
	_, err := api.SendDanmaku(dmReq, &api.BiliVerify{
		Csrf:     tool.biliJct,
		SessData: tool.sessData,
	})
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func strTrans(s string, name string) string {
	if len([]rune(name)) <= 22-len([]rune(s)) {
		return fmt.Sprintf(s, name)
	} else {
		return fmt.Sprintf(s, string([]rune(name)[len([]rune(name))-22+len([]rune(s)):len([]rune(name))]))
	}
}
