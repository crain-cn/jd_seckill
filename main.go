package main

import (
	"errors"
	"fmt"
	"github.com/Albert-Zhan/httpc"
	"github.com/tidwall/gjson"
	"jd_seckill/common"
	"jd_seckill/conf"
	"jd_seckill/seckill"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

var client *httpc.HttpClient

var cookieJar *httpc.CookieJar

var config *conf.Config

var wg *sync.WaitGroup

func init()  {
	//客户端设置初始化
	client=httpc.NewHttpClient()
	cookieJar=httpc.NewCookieJar()
	client.SetCookieJar(cookieJar)
	client.SetRedirect(func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	})
	//配置文件初始化
	confFile:="./conf.ini"
	if !common.Exists(confFile) {
		log.Println("配置文件不存在，程序退出")
		os.Exit(0)
	}
	config=&conf.Config{}
	config.InitConfig(confFile)

	wg=new(sync.WaitGroup)
	wg.Add(1)
}

func main()  {
	runtime.GOMAXPROCS(runtime.NumCPU())

	//用户登录
	user:= seckill.NewUser(client,config)
	wlfstkSmdl,err:=user.QrLogin()
	if err!=nil{
		os.Exit(0)
	}
	ticket:=""
	for  {
		ticket,err=user.QrcodeTicket(wlfstkSmdl)
		if err==nil && ticket!=""{
			break
		}
		time.Sleep(2*time.Second)
	}
	_,err=user.TicketInfo(ticket)
	if err==nil {
		log.Println("登录成功")
		//刷新用户状态和获取用户信息
		if status:=user.RefreshStatus();status==nil {
			userInfo,_:=user.GetUserInfo()
			log.Println("用户:"+userInfo)
			//开始预约,预约过的就重复预约
			seckill1 := seckill.NewSeckill(client,config)
			seckill1.MakeReserve()
			//等待抢购/开始抢购
			nowLocalTime:=time.Now().UnixNano()/1e6
			jdTime,_:=getJdTime(config.Read("config","sku_id"))
			buyDate := time.Unix(jdTime, 0).Format("2006-01-02 15:04:05")
			timerTime :=jdTime - nowLocalTime
			log.Println(fmt.Sprintf("正在等待到达设定时间:%s，设置等等【%d】毫秒开始抢购",buyDate,timerTime))
			if timerTime<=0 {
				log.Println("请设置抢购时间")
				os.Exit(0)
			}
			// time.Sleep(time.Duration(timerTime)*time.Millisecond)
			//开启任务
			log.Println("时间到达，开始执行……")
			start(seckill1,5)
			wg.Wait()
		}
	}else{
		log.Println("登录失败")
	}
}

func getJdTime(sku string) (int64,error) {
	req:=httpc.NewRequest(client)
	req.SetHeader("User-Agent","Mozilla/5.0 (iPhone; CPU iPhone OS 13_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148")
	resp,body,err:=req.SetUrl(fmt.Sprintf("https://item-soa.jd.com/getWareBusiness?skuId=%s",sku)).SetMethod("get").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("获取京东服务器时间失败")
		return 0,errors.New("获取京东服务器时间失败")
	}
	serverTime := gjson.Get(body,"yuyueInfo.buyTime").String()
	if serverTime == "" {
		log.Println("获取京东服务器时间失败")
		return 0,errors.New("获取京东服务器时间失败")
	}
	timestr := strings.Split(serverTime,"-2022")
	if len(timestr) < 1 {
		log.Println("获取京东服务器时间失败")
		return 0,errors.New("获取京东服务器时间失败")
	}
	loc, _ := time.LoadLocation("Local")
	t,_:=time.ParseInLocation("2006-01-02 15:04",timestr[0],loc)
	buyTime:=t.UnixNano()/1e6
	return buyTime - 1,nil
}

func start(seckill1 *seckill.Seckill,taskNum int)  {
	for i:=1;i<=taskNum;i++ {
		go func(seckill2 *seckill.Seckill) {
			seckill2.RequestSeckillUrl()
			seckill2.SeckillPage()
			seckill2.SubmitSeckillOrder()
		}(seckill1)
	}
}