package main

import (
    "bytes"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

    "github.com/codegangsta/cli"
	"github.com/gogo/protobuf/proto"
	log "github.com/golang/glog"
	"github.com/mesos/mesos-go/auth"
	"github.com/mesos/mesos-go/auth/sasl"
	mesos "github.com/mesos/mesos-go/mesosproto"
	util "github.com/mesos/mesos-go/mesosutil"
	sched "github.com/mesos/mesos-go/scheduler"
	"golang.org/x/net/context"
)

var CPUS_PER_EXECUTOR float64
var MEM_PER_EXECUTOR float64

type DockerScheduler struct {
	executor      *mesos.ExecutorInfo
	tasksLaunched int
	tasksFinished int
	tasksErrored  int
	totalTasks    int
}

func newDockerScheduler(exec *mesos.ExecutorInfo, total int) *DockerScheduler {
	return &DockerScheduler{
		executor:   exec,
		totalTasks: total,
	}
}

func getIp() string {
    cmd := exec.Command("/bin/hostname", "-i")

    var out bytes.Buffer
    cmd.Stdout = &out

    err := cmd.Run()
    if err != nil {
        panic(err)
    }

    ip := out.String()
    ips := strings.Split(ip, " ")
    return ips[0]
}

func (sched *DockerScheduler) Registered(driver sched.SchedulerDriver, frameworkId *mesos.FrameworkID, masterInfo *mesos.MasterInfo) {
	log.Infoln("Framework Registered with Master ", masterInfo)
}

func (sched *DockerScheduler) Reregistered(driver sched.SchedulerDriver, masterInfo *mesos.MasterInfo) {
	log.Infoln("Framework Re-Registered with Master ", masterInfo)
	_, err := driver.ReconcileTasks([]*mesos.TaskStatus{})
	if err != nil {
		log.Errorf("failed to request task reconciliation: %v", err)
	}
}

func (sched *DockerScheduler) Disconnected(sched.SchedulerDriver) {
	log.Warningf("disconnected from master")
}

func (sched *DockerScheduler) ResourceOffers(driver sched.SchedulerDriver, offers []*mesos.Offer) {

	if (sched.tasksLaunched - sched.tasksErrored) >= sched.totalTasks {
		log.Info("decline all of the offers since all of our tasks are already launched")
		ids := make([]*mesos.OfferID, len(offers))
		for i, offer := range offers {
			ids[i] = offer.Id
		}

        driver.LaunchTasks(ids, []*mesos.TaskInfo{}, &mesos.Filters{RefuseSeconds: proto.Float64(120)})
		return
	}
	for _, offer := range offers {
		cpuResources := util.FilterResources(offer.Resources, func(res *mesos.Resource) bool {
			return res.GetName() == "cpus"
		})
		cpus := 0.0
		for _, res := range cpuResources {
			cpus += res.GetScalar().GetValue()
		}

		memResources := util.FilterResources(offer.Resources, func(res *mesos.Resource) bool {
			return res.GetName() == "mem"
		})
		mems := 0.0
		for _, res := range memResources {
			mems += res.GetScalar().GetValue()
		}

		log.Infoln("Received Offer <", offer.Id.GetValue(), "> with cpus=", cpus, " mem=", mems)

		remainingCpus := cpus
		remainingMems := mems

		// account for executor resources if there's not an executor already running on the slave
		if len(offer.ExecutorIds) == 0 {
			remainingCpus -= CPUS_PER_EXECUTOR
			remainingMems -= MEM_PER_EXECUTOR
		}

		var tasks []*mesos.TaskInfo
		for (sched.tasksLaunched-sched.tasksErrored) < sched.totalTasks &&
			CPUS_PER_EXECUTOR <= remainingCpus &&
			MEM_PER_EXECUTOR <= remainingMems {

			sched.tasksLaunched++

			taskId := &mesos.TaskID{
				Value: proto.String(strconv.Itoa(sched.tasksLaunched)),
			}

			task := &mesos.TaskInfo{
				Name:     proto.String("go-task-" + taskId.GetValue()),
				TaskId:   taskId,
				SlaveId:  offer.SlaveId,
				Executor: sched.executor,
				Resources: []*mesos.Resource{
					util.NewScalarResource("cpus", CPUS_PER_EXECUTOR),
					util.NewScalarResource("mem", MEM_PER_EXECUTOR),
				},
			}
			log.Infof("Prepared task: %s with offer %s for launch\n", task.GetName(), offer.Id.GetValue())

			tasks = append(tasks, task)
			remainingCpus -= CPUS_PER_EXECUTOR
			remainingMems -= MEM_PER_EXECUTOR
		}
		log.Infoln("Launching ", len(tasks), "tasks for offer", offer.Id.GetValue())
		driver.LaunchTasks([]*mesos.OfferID{offer.Id}, tasks, &mesos.Filters{RefuseSeconds: proto.Float64(120)})
	}
}

func (sched *DockerScheduler) StatusUpdate(driver sched.SchedulerDriver, status *mesos.TaskStatus) {
	log.Infoln("Status update: task", status.TaskId.GetValue(), " is in state ", status.State.Enum().String())
	if status.GetState() == mesos.TaskState_TASK_FINISHED {
		sched.tasksFinished++
		driver.ReviveOffers()
	}

	if sched.tasksFinished >= sched.totalTasks {
		log.Infoln("Total tasks completed, stopping framework.")
		driver.Stop(false)
	}

	if status.GetState() == mesos.TaskState_TASK_LOST ||
		status.GetState() == mesos.TaskState_TASK_KILLED ||
		status.GetState() == mesos.TaskState_TASK_FAILED ||
		status.GetState() == mesos.TaskState_TASK_ERROR {
		sched.tasksErrored++
	}
}

func (sched *DockerScheduler) OfferRescinded(_ sched.SchedulerDriver, oid *mesos.OfferID) {
	log.Errorf("offer rescinded: %v", oid)
}
func (sched *DockerScheduler) FrameworkMessage(_ sched.SchedulerDriver, eid *mesos.ExecutorID, sid *mesos.SlaveID, msg string) {
	log.Errorf("framework message from executor %q slave %q: %q", eid, sid, msg)
}
func (sched *DockerScheduler) SlaveLost(_ sched.SchedulerDriver, sid *mesos.SlaveID) {
	log.Errorf("slave lost: %v", sid)
}
func (sched *DockerScheduler) ExecutorLost(_ sched.SchedulerDriver, eid *mesos.ExecutorID, sid *mesos.SlaveID, code int) {
	log.Errorf("executor %q lost on slave %q code %d", eid, sid, code)
}
func (sched *DockerScheduler) Error(_ sched.SchedulerDriver, err string) {
	log.Errorf("Scheduler received error: %v", err)
}

func init() {
	log.Infoln("Initializing the Example Scheduler...")
}

func prepareExecutorInfo(dockerCmd []string) *mesos.ExecutorInfo {
	executorCommand := "docker " + strings.Join(dockerCmd[:], " ")

	// Create mesos scheduler driver.
	return &mesos.ExecutorInfo{
		ExecutorId: util.NewExecutorID("default"),
		Name:       proto.String("docker-mesos"),
		Source:     proto.String("go_test"),
		Command: &mesos.CommandInfo{
			Value:  proto.String(executorCommand),
		},
		Resources: []*mesos.Resource{
			util.NewScalarResource("cpus", CPUS_PER_EXECUTOR),
			util.NewScalarResource("mem", MEM_PER_EXECUTOR),
		},
	}
}

func parseIP(address string) net.IP {
	addr, err := net.LookupIP(address)
	if err != nil {
		log.Fatal(err)
	}
	if len(addr) < 1 {
		log.Fatalf("failed to parse IP from address '%v'", address)
	}
	return addr[0]
}

func main() {

    // Commandline options
    app := cli.NewApp()
    app.Name = "docker-mesos"
    app.Version = "0.0.1"
    app.Usage = "Runs docker containers on mesos."
    app.Flags = []cli.Flag{
        cli.IntFlag{
            Name:  "copies, c",
            Value: 1,
            Usage: "Specify the number of copies to run.",
        },
        cli.Float64Flag{
            Name:  "cpus",
            Value: 0.1,
            Usage: "Maximum cpus required for task.",
        },
        cli.Float64Flag{
            Name:  "memory",
            Value: 500,
            Usage: "Maximum memory to allocate to the task.",
        },
        cli.StringFlag{
            Name:  "mesos_authentication_secret_file",
            Value: "",
            Usage: "Absolute path for mesos authentication file.",
        },
        cli.StringFlag{
            Name:  "mesos_authentication_principal",
            Value: "",
            Usage: "Provide mesos auth principle",
        },
        cli.StringFlag{
            Name:  "mesos_authentication_provider",
            Value: sasl.ProviderName,
            Usage: "Some usage",
        },
        cli.StringFlag{
            Name:  "mesos-master",
            Value: "10.85.5.50:5050",
            Usage: "Mesos master ip and port",
        },

    }

    app.Action = run
    app.Run(os.Args)
}

func run(ctx *cli.Context) {

    // Initialize all the commandline flags/variables
    mesosAuthPrincipal := ctx.String("mesos_authentication_principal")
    mesosAuthSecretFile := ctx.String("mesos_authentication_secret_file")
    authProvider := ctx.String("mesos_authentication_provider")
    master := ctx.String("mesos-master")
    copies := ctx.Int("copies")
    CPUS_PER_EXECUTOR = ctx.Float64("cpus")
    MEM_PER_EXECUTOR = ctx.Float64("memory")

    // Get the actual docker command to run
    var cmdArgs []string

    for _, arg := range ctx.Args() {
        cmdArgs = append(cmdArgs, arg)
    }

    // build command executor
	exec := prepareExecutorInfo(cmdArgs)

	fwinfo := &mesos.FrameworkInfo{
		User: proto.String(os.Getenv("USER")),
		Name: proto.String("docker-mesos"),
	}

    cred := (*mesos.Credential)(nil)
	if mesosAuthPrincipal != "" {
		fwinfo.Principal = proto.String(mesosAuthPrincipal)
		cred = &mesos.Credential{
			Principal: proto.String(mesosAuthPrincipal),
		}
		if mesosAuthSecretFile != "" {
			_, err := os.Stat(mesosAuthSecretFile)
			if err != nil {
				log.Fatal("missing secret file: ", err.Error())
			}
			secret, err := ioutil.ReadFile(mesosAuthSecretFile)
			if err != nil {
				log.Fatal("failed to read secret file: ", err.Error())
			}
			cred.Secret = proto.String(string(secret))
		}
	}
    ip := getIp()
	bindingAddress := parseIP(ip)

	config := sched.DriverConfig{
		Scheduler:      newDockerScheduler(exec, copies),
		Framework:      fwinfo,
		Master:         master,
		Credential:     cred,
		BindingAddress: bindingAddress,
		WithAuthContext: func(ctx context.Context) context.Context {
			ctx = auth.WithLoginProvider(ctx, authProvider)
			ctx = sasl.WithBindingAddress(ctx, bindingAddress)
			return ctx
		},
	}
	driver, err := sched.NewMesosSchedulerDriver(config)

	if err != nil {
		log.Errorln("Unable to create a SchedulerDriver ", err.Error())
	}

	if stat, err := driver.Run(); err != nil {
		log.Infof("Framework stopped with status %s and error: %s\n", stat.String(), err.Error())
		time.Sleep(2 * time.Second)
		os.Exit(1)
	}
	log.Infof("framework terminating")
}
