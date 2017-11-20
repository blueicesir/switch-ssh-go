package ssh

import (
	"golang.org/x/crypto/ssh"
	"net"
	"strings"
	"time"
)

/**
 * 封装的ssh session，包含原生的ssh.Ssssion及其标准的输入输出管道，同时记录最后的使用时间
 * @attr   session:原生的ssh session，in:绑定了session标准输入的管道，out:绑定了session标准输出的管道，lastUseTime:最后的使用时间
 * @author shenbowei
 */
type SSHSession struct {
	session     *ssh.Session
	in          chan string
	out         chan string
	brand       string
	lastUseTime time.Time
}

/**
 * 创建一个SSHSession，相当于SSHSession的构造函数
 * @param user ssh连接的用户名, password 密码, ipPort 交换机的ip和端口
 * @return 打开的SSHSession，执行的错误
 * @author shenbowei
 */
func NewSSHSession(user, password, ipPort string) (*SSHSession, error) {
	sshSession := new(SSHSession)
	if err := sshSession.createConnection(user, password, ipPort); err != nil {
		LogError("NewSSHSession createConnection error:%s", err.Error())
		return nil, err
	}
	if err := sshSession.muxShell(); err != nil {
		LogError("NewSSHSession muxShell error:%s", err.Error())
		return nil, err
	}
	if err := sshSession.start(); err != nil {
		LogError("NewSSHSession start error:%s", err.Error())
		return nil, err
	}
	sshSession.lastUseTime = time.Now()
	sshSession.brand = ""
	return sshSession, nil
}

/**
 * 获取最后的使用时间
 * @return time.Time
 * @author shenbowei
 */
func (this *SSHSession) GetLastUseTime() time.Time {
	return this.lastUseTime
}

/**
 * 更新最后的使用时间
 * @author shenbowei
 */
func (this *SSHSession) UpdateLastUseTime() {
	this.lastUseTime = time.Now()
}

/**
 * 连接交换机，并打开session会话
 * @param user ssh连接的用户名, password 密码, ipPort 交换机的ip和端口
 * @return 执行的错误
 * @author shenbowei
 */
func (this *SSHSession) createConnection(user, password, ipPort string) error {
	LogDebug("<Test> Begin connect")
	client, err := ssh.Dial("tcp", ipPort, &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		Timeout: 20 * time.Second,
		Config: ssh.Config{
			Ciphers: []string{"aes128-ctr", "aes192-ctr", "aes256-ctr", "aes128-gcm@openssh.com",
				"arcfour256", "arcfour128", "aes128-cbc", "aes256-cbc", "3des-cbc", "des-cbc",
			},
		},
	})
	if err != nil {
		LogError("SSH Dial err:%s", err.Error())
		return err
	}
	LogDebug("<Test> End connect")
	LogDebug("<Test> Begin new session")
	session, err := client.NewSession()
	if err != nil {
		LogError("NewSession err:%s", err.Error())
		return err
	}
	this.session = session
	LogDebug("<Test> End new session")
	return nil
}

/**
 * 启动多线程分别将返回的两个管道中的数据传输到会话的输入输出管道中
 * @return 错误信息error
 * @author shenbowei
 */
func (this *SSHSession) muxShell() error {
	defer func() {
		if err := recover(); err != nil {
			LogError("SSHSession muxShell err:%s", err)
		}
	}()
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if err := this.session.RequestPty("vt100", 80, 40, modes); err != nil {
		LogError("RequestPty error:%s", err)
		return err
	}
	w, err := this.session.StdinPipe()
	if err != nil {
		LogError("StdinPipe() error:%s", err.Error())
		return err
	}
	r, err := this.session.StdoutPipe()
	if err != nil {
		LogError("StdoutPipe() error:%s", err.Error())
		return err
	}

	in := make(chan string, 0)
	out := make(chan string, 0)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				LogError("Goroutine muxShell write err:%s", err)
			}
		}()
		for cmd := range in {
			_, err := w.Write([]byte(cmd + "\n"))
			if err != nil {
				LogDebug("Writer write err:%s", err.Error())
				return
			}
		}
	}()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				LogError("Goroutine muxShell read err:%s", err)
			}
		}()
		var (
			buf [65 * 1024]byte
			t   int
		)
		for {
			n, err := r.Read(buf[t:])
			if err != nil {
				LogDebug("Reader read err:%s", err.Error())
				return
			}
			t += n
			out <- string(buf[:t])
			t = 0
		}
	}()
	this.in = in
	this.out = out
	return nil
}

/**
 * 开始打开远程ssh登录shell，之后便可以执行指令
 * @return 错误信息error
 * @author shenbowei
 */
func (this *SSHSession) start() error {
	if err := this.session.Shell(); err != nil {
		LogError("Start shell error:%s", err.Error())
		return err
	}
	//等待登录信息输出
	this.ReadChannelExpect(1, "#", ">", "]")
	return nil
}

/**
 * 检查当前session是否可用
 * @return true:可用，false:不可用
 * @author shenbowei
 */
func (this *SSHSession) CheckSelf() bool {
	defer func() {
		if err := recover(); err != nil {
			LogError("SSHSession CheckSelf err:%s", err)
		}
	}()

	this.WriteChannel("\n")
	result := this.ReadChannelExpect(2, "#", ">", "]")
	if strings.Contains(result, "#") ||
		strings.Contains(result, ">") ||
		strings.Contains(result, "]") {
		return true
	}
	return false
}

/**
 * 获取当前SSH到的交换机的品牌
 * @return string （huawei,h3c,cisco）
 * @author shenbowei
 */
func (this *SSHSession) GetSSHBrand() string {
	defer func() {
		if err := recover(); err != nil {
			LogError("SSHSession GetSSHBrand err:%s", err)
		}
	}()
	if this.brand != "" {
		return this.brand
	}
	//显示版本后需要多一组空格，避免版本信息过多需要分页，导致分页指令第一个字符失效的问题
	this.WriteChannel("dis version", "show version", "     ")
	result := this.ReadChannelTiming(1)
	result = strings.ToLower(result)
	if strings.Contains(result, HUAWEI) {
		LogDebug("The switch brand is <huawei>.")
		this.brand = HUAWEI
	} else if strings.Contains(result, H3C) {
		LogDebug("The switch brand is <h3c>.")
		this.brand = H3C
	} else if strings.Contains(result, CISCO) {
		LogDebug("The switch brand is <cisco>.")
		this.brand = CISCO
	}
	return this.brand
}

/**
 * SSHSession的关闭方法，会关闭session和输入输出管道
 * @author shenbowei
 */
func (this *SSHSession) Close() {
	defer func() {
		if err := recover(); err != nil {
			LogError("SSHSession Close err:%s", err)
		}
	}()
	if err := this.session.Close(); err != nil {
		LogError("Close session err:%s", err.Error())
	}
	close(this.in)
	close(this.out)
}

/**
 * 向管道写入执行指令
 * @param cmds... 执行的命令（可多条）
 * @author shenbowei
 */
func (this *SSHSession) WriteChannel(cmds ...string) {
	LogDebug("WriteChannel <cmds=%v>", cmds)
	for _, cmd := range cmds {
		this.in <- cmd
	}
}

/**
 * 从输出管道中读取设备返回的执行结果，若输出流间隔超过maxIntervalTime或者包含expects中的字符便会返回
 * @param maxIntervalTime 输出管道的最大时间, expects...:期望得到的字符（可多个），得到便返回
 * @return 从输出管道读出的返回结果
 * @author shenbowei
 */
func (this *SSHSession) ReadChannelExpect(maxIntervalTime float32, expects ...string) string {
	LogDebug("ReadChannelExpect <maxIntervalTime=%f, expect=%v>", maxIntervalTime, expects)
	result := ""
	isDelay := false
ExitLoop:
	for {
		select {
		case sout := <-this.out:
			isDelay = false
			result = result + sout
			for _, expect := range expects {
				if strings.Contains(sout, expect) {
					break ExitLoop
				}
			}
		default:
			LogDebug("Channel is empty")
			//如果已经延迟过了，则直接返回
			if isDelay {
				break ExitLoop
			}
			time.Sleep(time.Duration(maxIntervalTime) * time.Second)
			isDelay = true
		}
	}
	return result
}

/**
 * 从输出管道中读取设备返回的执行结果，若输出流间隔超过maxIntervalTime便会返回
 * @param maxIntervalTime 输出管道的最大时间
 * @return 从输出管道读出的返回结果
 * @author shenbowei
 */
func (this *SSHSession) ReadChannelTiming(maxIntervalTime float32) string {
	LogDebug("ReadChannelTiming <maxIntervalTime = %f>", maxIntervalTime)
	result := ""
	isDelay := false
ExitLoop:
	for {
		select {
		case sout := <-this.out:
			isDelay = false
			result += sout
		default:
			LogDebug("Channel is empty")
			//如果已经延迟过了，则直接返回
			if isDelay {
				break ExitLoop
			}
			time.Sleep(time.Duration(maxIntervalTime) * time.Second)
			isDelay = true
		}
	}

	return result
}
