package main

import (
	"context"
	"fmt"
	"github.com/nsf/termbox-go"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DELAY = 3             // Delay between runs
const TERM_WIDTH = 80       // Display width
const VERSION = "1.0.1"     // Version
const LXC_INFO_TIMEOUT = 10 // Timeout in case lxc-info hangs

// Column offsets
const (
	OffsetName int = 0
	OffsetCpu      = 52
	OffsetMem      = 62
)

// Regexes for parsing lxc-info outpur
var elapsedRex *regexp.Regexp = regexp.MustCompile(`CPU use: [ ]+(\d+)`)
var memRex *regexp.Regexp = regexp.MustCompile(`Memory use: [ ]+(\d+)`)

//
// Container typre
type Container struct {
	Name          string
	LastCheckTime time.Time
	Cpu           uint64
	CpuPct        int
	Mem           uint64
}

// Sorting
type ByCpu []Container

func (b ByCpu) Len() int           { return len(b) }
func (b ByCpu) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByCpu) Less(i, j int) bool { return b[j].CpuPct < b[i].CpuPct }

type ByMem []Container

func (b ByMem) Len() int           { return len(b) }
func (b ByMem) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByMem) Less(i, j int) bool { return b[j].Mem < b[i].Mem }

//
// Coontainer map
type ContainerMap struct {
	sync.Mutex
	Containers map[string]Container
}

// Formatted memopry string
func (c *Container) MemPretty() string {
	switch {
	case c.Mem > 1073741824:
		return fmt.Sprintf("%0.2f GB", float64(c.Mem)/1073741824.0)
	case c.Mem > 1048576:
		return fmt.Sprintf("%0.2f MB", float64(c.Mem)/1048576.0)
	case c.Mem > 1024:
		return fmt.Sprintf("%0.2f KB", float64(c.Mem)/1024.0)
	default:
		return fmt.Sprintf("%d B", c.Mem)
	}
	return ""
}

func main() {
	fmt.Printf("lxc-top initializing...\n")
	lxcList() // Just test that we have containers and are running as roota
	err := termbox.Init()
	defer termbox.Close()
	if err != nil {
		panic(err)
	}
	quitChan := make(chan bool)
	sortChan := make(chan bool)
	go processEvents(quitChan, sortChan)
	go mainLoop(sortChan)
	<-quitChan
}

//
// main loop -- get container info, sort and display, delay
func mainLoop(sortChan chan bool) {
	containers := &ContainerMap{Containers: make(map[string]Container)}
	memsort := false
	for {
		lxcGetAll(containers)
		sortAndDisplay(containers, memsort)
		select {
		case memsort = <-sortChan:
		case <-time.After(DELAY * time.Second):
		}
	}
}

// Termbox event poller. Handle kbd input
func processEvents(quitChan chan bool, sortChan chan bool) {
	memsort := false
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Ch == 'q' {
				quitChan <- true
			} else if ev.Ch == 's' {
				memsort = !memsort
				sortChan <- memsort
			}

		case termbox.EventError:
			panic(ev.Err)

		case termbox.EventInterrupt:
			quitChan <- true
		}
	}
}

//
// Get info for all running containers concurrently
func lxcGetAll(containers *ContainerMap) {
	names := lxcList()
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if name != "" {
				lxcInfo(name, containers)
			}
		}(name)
	}
	wg.Wait()
}

//
// Sort and display as many containers as display dimensions will allow
func sortAndDisplay(c *ContainerMap, memsort bool) {
	var containers []Container

	for _, v := range c.Containers {
		containers = append(containers, v)
	}
	if memsort {
		sort.Sort(ByMem(containers))
	} else {
		sort.Sort(ByCpu(containers))
	}

	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	defer termbox.Flush()

	_, height := termbox.Size()

	cur_sort := "CPU"
	if memsort {
		cur_sort = "MEM"
	}

	tbClear(0, false)
	tbClear(1, false)
	tbPrint(0, 0, false, fmt.Sprintf("lxc-top v%s: 'q' to exit, 's' to toggle memory/cpu sort [%s]", VERSION, cur_sort))
	tbClear(2, true)
	tbPrint(OffsetName, 2, true, "NAME")
	tbPrint(OffsetCpu, 2, true, "CPU %")
	tbPrint(OffsetMem, 2, true, "MEM")

	for i, container := range containers {
		pos := i + 3
		if pos >= height {
			break
		}

		tbClear(pos, false)
		tbPrint(OffsetName, pos, false, container.Name)
		tbPrint(OffsetCpu, pos, false, fmt.Sprintf("%d", container.CpuPct))
		tbPrint(OffsetMem, pos, false, container.MemPretty())
	}

}

// List container names
func lxcList() []string {
	cmd := exec.Command("lxc-ls")
	out, err := cmd.CombinedOutput()
	if err != nil {
		Fatal("Unable to execute lxc-list (%s):\n%s", err.Error(), out)
	}
	if string(out) == "" {
		Fatal("lxc-info produced no output. Either no containers are running or you forgot to 'sudo lxc-top'")
	}
	ret := strings.Split(string(out), "\n")
	for i, name := range ret {
		ret[i] = strings.TrimSpace(name)
	}
	return ret
}

// Get container info with a timeout. Compute cpu usage, parse me usage, etc for a sibgle container
func lxcInfo(container string, containers *ContainerMap) {
	// Set up our run context
	ctx, cancel := context.WithTimeout(context.Background(), LXC_INFO_TIMEOUT*time.Second)
	defer cancel() // The cancel should be deferred so resources are cleaned up
	cmd := exec.CommandContext(ctx, "lxc-info", "-H", "-n", container)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		Fatal("Timed out getting lxc-info for container %s", container)
	}
	if err != nil {
		Fatal("Unable to get lxc-info for %s (%s):\n%s", container, err.Error(), out)
	}

	res := elapsedRex.FindAllStringSubmatch(string(out), -1)
	if len(res) == 0 {
		return // Assume container is stopped
	}
	cpu_time, _ := strconv.ParseUint(res[0][1], 10, 64)
	res = memRex.FindAllStringSubmatch(string(out), -1)
	if len(res) == 0 {
		Fatal("Unable to find mem use  in output:\n%s", out)
	}
	mem_used, _ := strconv.ParseUint(res[0][1], 10, 64)
	c := Container{Name: container, Mem: mem_used}
	c.LastCheckTime = time.Now()
	c.Cpu = cpu_time
	containers.Lock()
	defer containers.Unlock()
	old := containers.Containers[container]
	if old.Name != "" {
		dur := c.LastCheckTime.Sub(old.LastCheckTime)
		elapsed_cpu := c.Cpu - old.Cpu
		c.CpuPct = int(elapsed_cpu * 100 / uint64(dur))
	}
	containers.Containers[container] = c
}

// Fatal error
func Fatal(fmtStr string, args ...interface{}) {
	if termbox.IsInit {
		termbox.Close()
	}
	msg := fmt.Sprintf(fmtStr, args...)
	fmt.Printf("%s\n", msg)
	os.Exit(1)
}

// Clear a terminal line
func tbClear(y int, reverse bool) {
	tbPrint(0, y, reverse, strings.Repeat(" ", TERM_WIDTH))
}

// Print a string at x, y optionally reversed
func tbPrint(x, y int, reverse bool, msg string) {
	fg := termbox.ColorDefault
	bg := termbox.ColorDefault
	var attr termbox.Attribute
	if reverse {
		attr = termbox.AttrReverse
	}
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg|attr, bg|attr)
		x++
	}
}
