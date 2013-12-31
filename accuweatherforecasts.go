// AccuWeatherForecastsHourly project main.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//配置文件名称
const setting_file_name = "./settings.properties"

//go程数量
var complicate_count int = 0

//日志文件
var logger *log.Logger = nil
var savePath string = ""
var logFileName string = ""

//抓取数据保存路径
var dataSavePath_24 string = ""
var dataSavePath_1 string = ""

//apikey
var apikey string = ""

//城市请求key值
var cityInfo string = ""

//管道
var end chan int
var city chan City //主管道
//任务记数
var taskCount int

//任务完成数据
var finishCount int = 0
var l sync.Mutex

//心跳数组

//City类
type City struct {
	Id      string
	AccuKey string
	Path    string
}

/*************************/
//bool 代表 JSON booleans,
//float64 代表 JSON numbers,
//string 代表 JSON strings,
//nil 代表 JSON null.
/************************/
type Hour struct {
	DateTime            string
	WeatherIcon         float64
	IconPhrase          string
	Temperature         interface{}
	RealFeelTemperature interface{}
	RelativeHumidity    float64
	Unit                string
}
type Hourly struct {
	Hours []Hour
}

func main() {

	//读取配置文件
	settings, _ := readSetting(setting_file_name)
	savePath = settings["logPath"]
	dataSavePath_24 = settings["savePath_24"]
	dataSavePath_1 = settings["savePath_1"]
	complicate_count, _ = strconv.Atoi(settings["complicateNum"])
	apikey = settings["apiKey"]
	logFileName = settings["logFileName"]
	cityInfo = settings["cityInfo"]

	//配置日志保存文件
	t := time.Now()
	logger, _ = setLoggerSaveFile(savePath, savePath+logFileName+"."+strconv.Itoa(t.Year())+"-"+strconv.Itoa(int(t.Month()))+"-"+strconv.Itoa(t.Day()))
	makeSaveDirs()
	logger.Println("核心数：" + strconv.Itoa(runtime.NumCPU()) + "协程数：" + strconv.Itoa(complicate_count))
	//设置核心数
	runtime.GOMAXPROCS(runtime.NumCPU())
	cities, _ := readFileArray(cityInfo)
	taskCount = len(cities)
	city = make(chan City, complicate_count)
	defer close(city)
	end = make(chan int)
	defer close(end)
	var heart [1000]chan int
	go writeCitiesToChannel(city, cities)
	for i := 0; i < complicate_count; i++ {
		heart[i] = make(chan int)
		go startRequest(city, heart[i])
	}
	//go func() {
	//	time.Sleep(time.Second * 5)
	//	for i := 0; i < complicate_count; i++ {
	//		heart[i] <- 1
	//	}
	//}()
	if <-end > 0 {
		logger.Println("任务执行完成一次")
	}
}
func makeSaveDirs() {
	//创建24小时预报数据保存路径
	for i := 0; i < 100; i++ {
		err := os.MkdirAll(dataSavePath_24+strconv.Itoa(i), 0777)
		if err != nil {
			logger.Panicln("创建文件保存目录失败")
		}
	}
	//创建历史1小时预报数据保存目录
	for i := 0; i < 24; i++ {
		if i < 10 {
			err := os.MkdirAll(dataSavePath_1+"0"+strconv.Itoa(i), 0777)
			if err != nil {
				logger.Panicln("创建文件保存目录失败")
			}
		} else {
			err := os.MkdirAll(dataSavePath_1+strconv.Itoa(i), 0777)
			if err != nil {
				logger.Panicln("创建文件保存目录失败")
			}
		}
	}
}
func writeCitiesToChannel(city chan City, cities []City) {
	for i := 0; i < len(cities); i++ {
		city <- cities[i]
	}
	logger.Println("城市信息写入channel完成,启动结束计时")
	//启动任务结束计时
	go func() {
		time.Sleep(time.Second * 60 * 2)
		end <- 1
	}()
}

//发送http请求
func startRequest(ch chan City, heart chan int) {
	client := &http.Client{}
	var city City
	for {
		select {
		case city = <-ch:
			if finishCount == taskCount {
				runtime.Goexit()
			}
			if len(city.Id) == 0 || len(city.AccuKey) == 0 {
				continue
			}
			//resp, err := http.Get("http://apidev.accuweather.com/forecasts/v1/hourly/24hour/" + city.AccuKey + ".json?apiKey=" + apikey + "&language=en&details=true")
			resp, err := client.Get("http://apidev.accuweather.com/forecasts/v1/hourly/24hour/" + city.AccuKey + ".json?apiKey=" + apikey + "&language=en&details=true")
			if nil != err {
				logger.Println("城市：" + city.Id + "请求失败：" + city.AccuKey)
				ch <- city
				continue
			}
			body, err := ioutil.ReadAll(resp.Body)
			if nil != err && len(body) != 0 {
				logger.Println("获取内容失败！")
				ch <- city
				continue
			}
			resp.Body.Close()
			//save datas
			var hourly Hourly
			var save Hourly
			var history Hour
			json.Unmarshal(body, &hourly.Hours)
			for _, v := range hourly.Hours {
				data_Tmperature, _ := v.Temperature.(map[string]interface{})
				data_RealFeelTemperature, _ := v.RealFeelTemperature.(map[string]interface{})
				save.Hours = append(save.Hours, Hour{DateTime: v.DateTime, WeatherIcon: v.WeatherIcon, IconPhrase: v.IconPhrase, RelativeHumidity: v.RelativeHumidity, Temperature: data_Tmperature["Value"], RealFeelTemperature: data_RealFeelTemperature["Value"], Unit: data_Tmperature["Unit"].(string)})
				if k == 0 {
					history = save.Hours[0]
				}
			}
			//save future 24 hours forecast data
			data_24, err24 := json.Marshal(save)
			if err24 != nil {
				fmt.Println("data_24 json err:", err)
			}
			path_24 := dataSavePath_24 + city.Path + city.Id + ".json"
			file_24, err_24 := os.OpenFile(path_24, os.O_CREATE|os.O_RDWR, 0660)
			if nil != err_24 {
				logger.Println(city.Id + ".json打开文件失败！")
				continue
			} else {
				_, err_24 := file_24.Write(data_24)
				if nil != err_24 {
					logger.Println(city.Id + ".json 写入失败！")
				}
			}
			file_24.Close()
			//save future 24 hours first hour forecast date
			data_1, err_1 := json.Marshal(history)
			if err_1 != nil {
				logger.Panicln("data_1 json err", err_1)
			}
			dir := history.DateTime[11:13]
			path_1 := dataSavePath_1 + dir + "/" + city.Id + ".json"
			file_1, err_1 := os.OpenFile(path_1, os.O_CREATE|os.O_RDWR, 0660)
			if nil != err_1 {
				logger.Println(city.Id + ".json打开文件失败！")
				continue
			} else {
				_, err_1 := file_1.Write(data_1)
				if nil != err_1 {
					logger.Println(city.Id + ".json 写入失败！")
				}
			}
			file_1.Close()
			l.Lock()
			finishCount++
			fmt.Println(finishCount)
			l.Unlock()
		case <-heart:
		}
	}
}

//设置日志保存路径和文件文件名
func setLoggerSaveFile(filePath string, fileName string) (loger *log.Logger, err error) {
	dirErr := os.MkdirAll(filePath, 0700)
	if dirErr != nil {
		fmt.Println("日志文件目录创建失败！")
		return nil, dirErr
	} else {
		logfile, fileErr := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
		if fileErr != nil {
			fmt.Println("打开日志保存文件失败！")
			return nil, fileErr
		}
		var logger *log.Logger
		logger = log.New(logfile, "", log.Ldate|log.Ltime)
		return logger, nil
	}
}

//读取配置文件方法
func readSetting(fileName string) (setting map[string]string, err error) {
	//#开头的正则表达式
	reg := regexp.MustCompile(`^#.*`)
	settings := make(map[string]string)
	settingFile, err := os.OpenFile(fileName, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	settingReader := bufio.NewReader(settingFile)
	for {
		str, _, err := settingReader.ReadLine()
		if err != nil {
			if io.EOF == err {
				break
			} else {
				fmt.Println("读取配置文件错误")
				break
			}
		}
		content := string(str[:])
		if 0 == len(content) || "\r\n" == content || reg.FindAllString(content, -1) != nil {
			continue
		}
		items := strings.Split(strings.TrimSpace(content), "=")
		settings[items[0]] = items[1]
	}
	return settings, nil
}

//读入城市请求key值
func readFileArray(fileName string) (result []City, err error) {
	var cities = make([]City, 0)
	srcFile, err := os.OpenFile(fileName, os.O_RDONLY, 0440)
	if nil != err {
		logger.Println("打开城市信息文件失败")
		return nil, err
	}
	defer srcFile.Close()
	srcReader := bufio.NewReader(srcFile)
	for {
		str, _, err := srcReader.ReadLine()
		if nil != err {
			if io.EOF == err {
				break
			} else {
				logger.Println("读取城市信息文件发生错误")
			}
		}
		content := string(str[:])
		if 0 == len(content) || "\r\n" == content {
			continue
		}
		var city City
		items := strings.Split(strings.TrimSpace(content), ",")
		if len(items) == 2 {
			city.Id = items[0]
			city.AccuKey = items[1]
			bucket, _ := strconv.Atoi(items[0])
			city.Path = "/" + strconv.Itoa(bucket%100) + "/"
			cities = append(cities, city)
		}
	}
	return cities, nil
}
