package exper

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func getCPUSample() (idle, total uint64) {
	//读取/proc/stat内容
	contents, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	//读取内容转化为字符串
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		//将字符串以空白字符（\t, \n, \v, \f, \r, ’ ‘, U+0085 (NEL), U+00A0 (NBSP) 。）进行分割多个子串
		fields := strings.Fields(line)
		//如果第一列字符为“cpu”
		if fields[0] == "cpu" {
			//统计子串数量
			numFields := len(fields)
			for i := 1; i < numFields; i++ {
				val, err := strconv.ParseUint(fields[i], 10, 64)
				if err != nil {
					fmt.Println("Error: ", i, fields[i], err)
				}
				//将是CPU字符的所有子串相加
				total += val // tally up all the numbers to get total ticks
				if i == 4 {  // idle is the 5th field in the cpu line
					//第五列的数据赋值给idle
					idle = val
				}
			}
			return
		}
	}
	return
}

func ReadLine(lineNumber int) string {
	//读取/proc/meminfo内容
	file, _ := os.Open("/proc/meminfo")
	//按行读取所有内容
	fileScanner := bufio.NewScanner(file)
	lineCount := 1
	for fileScanner.Scan() {
		if lineCount == lineNumber {
			return fileScanner.Text()
		}
		lineCount++
	}
	defer file.Close()
	return ""
}

func handler(w http.ResponseWriter, r *http.Request) {
	//定义一个整数型切片
	var s []int
	//读取/proc/meminfo的第二行内容
	MemFree := ReadLine(2)
	//将读取的内容转化为字符串
	MemFree_lines := strings.Split(string(MemFree), "\n")
	//将字符串以空白字符（\t, \n, \v, \f, \r, ’ ‘, U+0085 (NEL), U+00A0 (NBSP) 。）进行分割多个子串
	for _, MemFree_line := range MemFree_lines {
		fields := strings.Fields(MemFree_line)
		//将第二列的内容转化为整数，并将这个整数追加到s切片中
		if MemFree_line, err := strconv.Atoi(fields[1]); err == nil {
			//fmt.Printf("%T, %v", MemFree_line, MemFree_line)
			s = append(s, MemFree_line)
		}
	}
	Buffers := ReadLine(4)
	Buffers_lines := strings.Split(string(Buffers), "\n")
	for _, Buffers_line := range Buffers_lines {
		fields := strings.Fields(Buffers_line)
		if Buffers_line, err := strconv.Atoi(fields[1]); err == nil {
			//fmt.Printf("%T, %v", Buffers_line, Buffers_line)
			s = append(s, Buffers_line)
		}
	}
	Cached := ReadLine(4)
	Cached_lines := strings.Split(string(Cached), "\n")
	for _, Cached_line := range Cached_lines {
		fields := strings.Fields(Cached_line)
		if Cached_line, err := strconv.Atoi(fields[1]); err == nil {
			//fmt.Printf("%T, %v", Cached_line, Cached_line)
			s = append(s, Cached_line)
		}
	}
	MemTotal := ReadLine(1)
	MemTotal_lines := strings.Split(string(MemTotal), "\n")
	for _, MemTotal_line := range MemTotal_lines {
		fields := strings.Fields(MemTotal_line)
		if MemTotal_line, err := strconv.Atoi(fields[1]); err == nil {
			//fmt.Printf("%T, %v", MemTotal_line, MemTotal_line)
			s = append(s, MemTotal_line)

		}
	}

	idle0, total0 := getCPUSample()
	time.Sleep(3 * time.Second)
	idle1, total1 := getCPUSample()

	idleTicks := float64(idle1 - idle0)
	totalTicks := float64(total1 - total0)
	//计算cpu利用率
	cpuUsage := 100 * (totalTicks - idleTicks) / totalTicks
	//计算内存的使用率及剩余量
	memoryused := (s[0] + s[1] + s[2])
	memoryfreeused := (s[3] - memoryused)
	//页面显示内容
	fmt.Fprintf(w, "#HELP node_memory_guest_seconds\n node_memory{key=\"used\"}\t%v\n node_memory{key=\"free\"}\t%v\n#node_CPU_guest_seconds\n node_cpu{key=\"usage\"}\t%v\n", memoryused, memoryfreeused, cpuUsage)

}
