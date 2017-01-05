package loadBalancer

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"
)

var i int = -1
var m *sync.Mutex = new(sync.Mutex)

type LoadBalancer struct {
	CurrentWeight int
	WellNodes     []map[string]string
	SickNodes     []map[string]string
}

func swap(a_p *int, b_p *int) {
	temp := *a_p
	*a_p = *b_p
	*b_p = temp
}

func gcd(a int, b int) int {
	if a < b {
		a_p := new(int)
		b_p := new(int)
		a_p = &a
		b_p = &b
		swap(a_p, b_p)
	}
	if b == 0 {
		return a
	} else {
		return gcd(b, a%b)
	}
}

// 这里的长度使用计算数组的长度最好，不要使用写死的方式，这会造成后期bug的存在
func Ngcd(arr []int, n int) int {
	if n == 1 {
		return arr[0]
	}
	return gcd(arr[n-1], Ngcd(arr, n-1))
}

func Max(arr []int) int {
	var max int = 0
	for _, item := range arr {
		if item > max {
			max = item
		}
	}
	return max
}

func (p *LoadBalancer) WeightedRoundRobin() (destination int) {

	// TODO: 配置文件数据合法性校验
	// 返回的整数是节点数组的索引
	// 权重的值如果格式非法，要给出简单的处理策略：
	// 1、如果权重的字符串是“空串”，取权重中的最小值处理
	// 对于新加入的节点是否需要取最小权值，有待讨论！！！
	// 返回的数值，是更新过的WellNodes数组中的索引值

	destination = -1
	node_pos := -1
	var addrs []string
	var weights []int

	if len(p.WellNodes) > 0 {
		for _, item := range p.WellNodes {
			addrs = append(addrs, item["addr"])
			weight, err := strconv.Atoi(item["weight"])
			if err != nil {
				log.Println(err)
				weight = 0
			}
			// TODO: 后面需要将0替换为权重最小的那个值
			weights = append(weights, weight)
		}

		for node_pos <= -1 {
			i = (i + 1) % (len(addrs))
			if i == 0 {
				p.CurrentWeight = p.CurrentWeight - Ngcd(weights, len(weights))
				if p.CurrentWeight <= 0 {
					p.CurrentWeight = Max(weights)
					if p.CurrentWeight == 0 {
						// 没有服务器被选中, 设定-1表示所有的服务器不可用
						destination = -1
						return
					}
				}
			}
			if weights[i] >= p.CurrentWeight {
				// 返回选中的服务器
				destination = i
				node_pos = destination
			} else {
				node_pos = -1
			}
		}
		return
	} else {
		return -1
	}

}


// 在各种服务心跳的检测方式中，暂时使用“TCP连接测试”的方式来实现健康检测
// delay_loop: 3s 隔多长时间做一次健康检测，单位为秒
// connect_timeout: 3s 连接超时时间，单位为秒
// nb_get_retry 检测失败后的重试次数，如果达到重试次数仍然失败，将节点从服务器池中移除
// delay_before_retry 失败重试的间隔时间，单位为秒

func (p *LoadBalancer) RetryDial(addr string) {

	// /*
	// 如果在这个函数里还不能通过服务健康检测，就更新p.WellNodes
	// 移除故障节点到“故障节点数组”中，留待后期的“服务重拾”
	// 目前只尝试一次，如果失败了就不再尝试
	// */

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		m.Lock()
		for index, item := range p.WellNodes {
			if item["addr"] == addr {
				p.SickNodes = append(p.SickNodes, p.WellNodes[index])
				p.WellNodes = append(p.WellNodes[:index], p.WellNodes[index+1:]...)
				break
			}
		}
		m.Unlock()
		log.Printf("节点【%v】的服务故障，该服务节点已从服务器池中移除！\n", addr)
	} else {
		conn.Close()
	}

}

func (p *LoadBalancer) TCPCheck() {
	ticker := time.NewTicker(3 * time.Second)
	// 因为ticker使用的是无缓冲的channel，必须启用一个goroutine来保证后续代码的执行
	go func() {
		for tick := range ticker.C {
			_ = tick
			if len(p.WellNodes) > 0 {
				for index, node := range p.WellNodes {
					// 这里的goroutine只负责将每个节点送到“检测流程”去, 不负责具体的检测工作
					// 具体的检测工作留在LoadBalancer的其他方法中
					go func(index int, node map[string]string) {
						conn, err := net.DialTimeout("tcp", node["addr"], 3*time.Second)
						if err != nil {
							log.Printf("%v [will be retry]\n", err)
							p.RetryDial(node["addr"])
						} else {
							defer conn.Close()
						}
						//fmt.Println(node)
					}(index, node)
				}
			} else {
				//fmt.Println(p.WellNodes)
				//fmt.Println(p.SickNodes)
				log.Println("没有可以用来服务的节点，请排查相关原因")
			}
		}
	}()
}


func (p *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	///*
	// 拦截器在这里实现！
	// */

	if r.URL.Path != "/favicon.ico" {
		// 在此处实现负载调度任务,将请求反向代理到后面的“业务服务器池”中去
		des := p.WeightedRoundRobin()
		if des == -1 {
			i = -1
			log.Println("没有可用的服务节点，请检查各节点的服务是否正常")
		} else {
			log.Printf("被选中的运行良好的服务节点号为【%v】，节点地址为【%v】请求将会分发到该节点上\n", des, p.WellNodes[des]["addr"])
			remote, err := url.Parse("http://" + p.WellNodes[des]["addr"])
			if err != nil {
				panic(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(remote)
			proxy.ServeHTTP(w, r)
		}
	}
}

