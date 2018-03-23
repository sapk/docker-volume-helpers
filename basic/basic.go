package basic

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sapk/docker-volume-helpers/driver"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/docker/go-plugins-helpers/volume"
)

//Mountpoint represent a docker volume mountpoint
type Mountpoint struct {
	Path        string `json:"path"`
	Connections int    `json:"connections"`
}

//GetPath get path of mount
func (d *Mountpoint) GetPath() string {
	return d.Path
}

//GetConnections get number of connection on mount
func (d *Mountpoint) GetConnections() int {
	return d.Connections
}

//SetConnections set number of connection on mount
func (d *Mountpoint) SetConnections(n int) {
	d.Connections = n
}

//Volume represent a docker volume
type Volume struct {
	VolumeURI   string `json:"voluri"`
	Mount       string `json:"mount"`
	Connections int    `json:"connections"`
}

//GetMount get mount of volume
func (v *Volume) GetMount() string {
	return v.Mount
}

//GetRemote get remote definition of volume
func (v *Volume) GetRemote() string {
	return v.VolumeURI
}

//GetConnections get number of connection on volume
func (v *Volume) GetConnections() int {
	return v.Connections
}

//SetConnections set number of connection on volume
func (v *Volume) SetConnections(n int) {
	v.Connections = n
}

func (v *Volume) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"TODO": "List",
	}
}

//Driver the global driver responding to call
type Driver struct {
	Lock         sync.RWMutex
	Persistence  *viper.Viper
	Volumes      map[string]*Volume
	Mounts       map[string]*Mountpoint
	Config       DriverConfig
	EventHandler DriverEventHandler
}

//DriverConfig contains configration of driver
type DriverConfig struct {
	Version       int
	Root          string
	MountUniqName bool
	Folder        string
}

//DriverEventHandler contains function to execute on event
type DriverEventHandler struct {
	IsValidURI    func(string) bool
	OnInit        func(*Driver) (*volume.MountResponse, error)
	OnMountVolume func(*Driver, driver.Volume, driver.Mount, *volume.MountRequest) (*volume.MountResponse, error)
}

//GetVolumes list volumes of driver
func (d *Driver) GetVolumes() map[string]driver.Volume {
	vi := make(map[string]driver.Volume, len(d.Volumes))
	for k, i := range d.Volumes {
		vi[k] = i
	}
	return vi
}

//GetMounts list mounts of driver
func (d *Driver) GetMounts() map[string]driver.Mount {
	mi := make(map[string]driver.Mount, len(d.Mounts))
	for k, i := range d.Mounts {
		mi[k] = i
	}
	return mi
}

//GetLock list lock of driver
func (d *Driver) GetLock() *sync.RWMutex {
	return &d.Lock
}

//Create create and init the requested volume
func (d *Driver) Create(r *volume.CreateRequest) error {
	logrus.Debugf("Entering Create: name: %s, options %v", r.Name, r.Options)

	if r.Options == nil || r.Options["voluri"] == "" {
		return fmt.Errorf("voluri option required")
	}
	r.Options["voluri"] = strings.Trim(r.Options["voluri"], "\"")
	if !d.EventHandler.IsValidURI(r.Options["voluri"]) {
		return fmt.Errorf("voluri option is malformated")
	}

	d.GetLock().Lock()
	defer d.GetLock().Unlock()

	v := &Volume{
		VolumeURI:   r.Options["voluri"],
		Mount:       GetMountName(d, r),
		Connections: 0,
	}

	if _, ok := d.Mounts[v.Mount]; !ok { //This mountpoint doesn't allready exist -> create it
		m := &Mountpoint{
			Path:        filepath.Join(d.Config.Root, v.Mount),
			Connections: 0,
		}

		_, err := os.Lstat(m.Path) //Create folder if not exist. This will also failed if already exist
		if os.IsNotExist(err) {
			if err = os.MkdirAll(m.Path, 0700); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		isempty, err := FolderIsEmpty(m.Path)
		if err != nil {
			return err
		}
		if !isempty {
			return fmt.Errorf("%v already exist and is not empty", m.Path)
		}
		d.Mounts[v.Mount] = m
	}

	d.Volumes[r.Name] = v
	logrus.Debugf("Volume Created: %v", v)
	return d.SaveConfig()
}

//List Volumes handled by these driver
func (d *Driver) List() (*volume.ListResponse, error) {
	return driver.List(d)
}

//Get get info on the requested volume
func (d *Driver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	v, m, err := driver.Get(d, r.Name)
	if err != nil {
		return nil, err
	}
	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Status: v.GetStatus(), Mountpoint: m.GetPath()}}, nil
}

//Remove remove the requested volume
func (d *Driver) Remove(r *volume.RemoveRequest) error {
	return driver.Remove(d, r.Name)
}

//Unmount unmount the requested volume
func (d *Driver) Unmount(r *volume.UnmountRequest) error {
	return driver.Unmount(d, r.Name)
}

//Capabilities Send capabilities of the local driver
func (d *Driver) Capabilities() *volume.CapabilitiesResponse {
	return driver.Capabilities()
}

//RunCmd run deamon in context of this gvfs drive with custome env
func (d *Driver) RunCmd(cmd string) error {
	logrus.Debugf(cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		logrus.Warnf("Error: %s", err)
	}
	logrus.Warnf("Output: %s", out)
	return err
}

//Persistence represent struct of Persistence file
type Persistence struct {
	Version int                    `json:"version"`
	Volumes map[string]*Volume     `json:"volumes"`
	Mounts  map[string]*Mountpoint `json:"mounts"`
}

//SaveConfig stroe config/state in file
func (d *Driver) SaveConfig() error {
	fi, err := os.Lstat(d.Config.Folder)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(d.Config.Folder, 0700); err != nil {
			return fmt.Errorf("SaveConfig: %s", err)
		}
	} else if err != nil {
		return fmt.Errorf("SaveConfig: %s", err)
	}
	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("SaveConfig: %v already exist and it's not a directory", d.Config.Root)
	}
	b, err := json.Marshal(Persistence{Version: d.Config.Version, Volumes: d.Volumes, Mounts: d.Mounts})
	if err != nil {
		logrus.Warn("Unable to encode Persistence struct, %v", err)
	}
	//logrus.Debug("Writing Persistence struct, %v", b, d.Volumes)
	err = ioutil.WriteFile(d.Config.Folder+"/persistence.json", b, 0600)
	if err != nil {
		logrus.Warn("Unable to write Persistence struct, %v", err)
		return fmt.Errorf("SaveConfig: %s", err)
	}
	return nil
}

//Path get path of the requested volume
func (d *Driver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	_, m, err := driver.Get(d, r.Name)
	if err != nil {
		return nil, err
	}
	return &volume.PathResponse{Mountpoint: m.GetPath()}, nil
}

//Mount mount the requested volume
func (d *Driver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.Debugf("Entering Mount: %v", r)

	v, m, err := driver.MountExist(d, r.Name)
	if err != nil {
		return nil, err
	}
	if m != nil && m.GetConnections() > 0 {
		return &volume.MountResponse{Mountpoint: m.GetPath()}, nil
	}

	d.GetLock().Lock()
	defer d.GetLock().Unlock()

	resp, err := d.EventHandler.OnMountVolume(d, v, m, r)
	if err != nil {
		return nil, err
	}
	//time.Sleep(3 * time.Second)
	driver.AddN(1, v, m)
	return resp, d.SaveConfig()
}

//Init load configuration and serve response to API call
func Init(config DriverConfig, eventHandler DriverEventHandler) *Driver {
	logrus.Debugf("Init basic driver at %s, UniqName: %v", config.Root, config.MountUniqName)
	d := &Driver{
		Config:       config,
		Persistence:  viper.New(),
		Volumes:      make(map[string]*Volume),
		Mounts:       make(map[string]*Mountpoint),
		EventHandler: eventHandler,
	}

	d.Persistence.SetDefault("volumes", map[string]*Volume{})
	d.Persistence.SetDefault("mounts", map[string]*Mountpoint{})
	d.Persistence.SetConfigName("persistence")
	d.Persistence.SetConfigType("json")
	d.Persistence.AddConfigPath(d.Config.Folder)
	if err := d.Persistence.ReadInConfig(); err != nil { // Handle errors reading the config file
		logrus.Warn("No persistence file found, I will start with a empty list of volume.", err)
	} else {
		logrus.Debug("Retrieving volume list from persistence file.")

		var version int
		err := d.Persistence.UnmarshalKey("version", &version)
		if err != nil || version != d.Config.Version {
			logrus.Warn("Unable to decode version of persistence, %v", err)
			d.Volumes = make(map[string]*Volume)
			d.Mounts = make(map[string]*Mountpoint)
		} else { //We have the same version
			err := d.Persistence.UnmarshalKey("volumes", &d.Volumes)
			if err != nil {
				logrus.Warn("Unable to decode into struct -> start with empty list, %v", err)
				d.Volumes = make(map[string]*Volume)
			}
			err = d.Persistence.UnmarshalKey("mounts", &d.Mounts)
			if err != nil {
				logrus.Warn("Unable to decode into struct -> start with empty list, %v", err)
				d.Mounts = make(map[string]*Mountpoint)
			}
		}
	}
	return d
}
