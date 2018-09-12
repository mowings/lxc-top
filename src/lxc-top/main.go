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

const DELAY = 3

var quit bool
var memsort bool

var elapsedRex *regexp.Regexp = regexp.MustCompile(`CPU use: [ ]+(\d+)`)
var memRex *regexp.Regexp = regexp.MustCompile(`Memory use: [ ]+(\d+)`)

type Container struct {
	Name          string
	LastCheckTime time.Time
	Elapsed       uint64
	ElapsedPct    int
	Mem           uint64
}

type ByCpu []Container

func (b ByCpu) Len() int           { return len(b) }
func (b ByCpu) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByCpu) Less(i, j int) bool { return b[j].ElapsedPct < b[i].ElapsedPct }

type ByMem []Container

func (b ByMem) Len() int           { return len(b) }
func (b ByMem) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByMem) Less(i, j int) bool { return b[j].Mem < b[i].Mem }

type ContainerMap struct {
	sync.Mutex
	Containers map[string]Container
}

func (c *Container) MemFmt() string {
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
	if err != nil {
		panic(err)
	}
	go processEvents()
	defer termbox.Close()
	containers := &ContainerMap{Containers: make(map[string]Container)}
	for {
		lxcGetAll(containers)
		sortAndDisplay(containers)
		delay()
		if quit {
			return
		}
	}
}

func delay() {
	last_sort := memsort
	for i := 0; i < 5*DELAY; i++ {
		if quit {
			return
		}
		if last_sort != memsort {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func processEvents() {
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Ch == 'q' {
				quit = true
			} else if ev.Ch == 's' {
				memsort = !memsort
			}

		case termbox.EventError:
			panic(ev.Err)

		case termbox.EventInterrupt:
			quit = true
		}
	}
}

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

func sortAndDisplay(c *ContainerMap) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
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

	tbPrint(0, 0, false, strings.Repeat(" ", 80))
	tbPrint(0, 0, false, fmt.Sprintf("'q' to exit, 's' to toggle memory/cpu sort [%s]", cur_sort))
	tbPrint(0, 2, true, strings.Repeat(" ", 80))
	tbPrint(0, 2, true, "NAME")
	tbPrint(52, 2, true, "CPU %")
	tbPrint(62, 2, true, "MEM")

	for i, container := range containers {
		pos := i + 3
		if pos >= height {
			break
		}

		tbPrint(0, pos, false, strings.Repeat(" ", 80))
		tbPrint(0, pos, false, container.Name)
		tbPrint(52, pos, false, fmt.Sprintf("%d", container.ElapsedPct))
		tbPrint(62, pos, false, container.MemFmt())
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
	return ret
}

func lxcInfo(container string, containers *ContainerMap) {
	if quit {
		return
	}
	// Set up our run context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	c.Elapsed = cpu_time
	containers.Lock()
	defer containers.Unlock()
	old := containers.Containers[container]
	if old.Name != "" {
		dur := c.LastCheckTime.Sub(old.LastCheckTime)
		elapsed_cpu := c.Elapsed - old.Elapsed
		c.ElapsedPct = int(elapsed_cpu * 100 / uint64(dur))
	}
	containers.Containers[container] = c
}

func Fatal(fmtStr string, args ...interface{}) {
	msg := fmt.Sprintf(fmtStr, args...)
	fmt.Printf("%s\n", msg)
	os.Exit(1)
}

func tbPrint(x, y int, reverse bool, msg string) {
	fg := termbox.ColorDefault
	bg := termbox.ColorDefault
	if reverse {
		fg |= termbox.AttrReverse
		bg |= termbox.AttrReverse
	}
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x++
	}
}
