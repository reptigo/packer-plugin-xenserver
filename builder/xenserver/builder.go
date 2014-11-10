package xenserver

import (
    "github.com/mitchellh/multistep"
    "github.com/mitchellh/packer/packer"
    "github.com/mitchellh/packer/common"
    "fmt"
    "log"
    "errors"
    "time"
    "strings"
    "os"
    commonssh "github.com/mitchellh/packer/common/ssh"
)


// Set the unique ID for this builder
const BuilderId = "packer.xenserver"


type config struct {
    common.PackerConfig `mapstructure:",squash"`

    Username        string      `mapstructure:"username"`
    Password        string      `mapstructure:"password"`
    HostIp          string      `mapstructure:"host_ip"`
    IsoUrl          string      `mapstructure:"iso_url"`

    InstanceName    string      `mapstructure:"instance_name"`
    RootDiskSize    string      `mapstructure:"root_disk_size"`
    CloneTemplate   string      `mapstructure:"clone_template"`
    IsoUuid         string      `mapstructure:"iso_uuid"`
    SrUuid          string      `mapstructure:"sr_uuid"`
    NetworkUuid     string      `mapstructure:"network_uuid"`

    VncPortMin      uint        `mapstructure:"vnc_port_min"`
    VncPortMax      uint        `mapstructure:"vnc_port_max"`

    BootCommand     []string    `mapstructure:"boot_command"`
    RawBootWait     string      `mapstructure:"boot_wait"`

    BootWait        time.Duration ``
    sshWaitTimeout  time.Duration ``

    ISOChecksum     string      `mapstructure:"iso_checksum"`
    ISOChecksumType string      `mapstructure:"iso_checksum_type"`
    ISOUrls         []string    `mapstructure:"iso_urls"`
    ISOUrl          string      `mapstructure:"iso_url"`

    HTTPDir         string      `mapstructure:"http_directory"`
    HTTPPortMin     uint        `mapstructure:"http_port_min"`
    HTTPPortMax     uint        `mapstructure:"http_port_max"`

    LocalIp         string      `mapstructure:"local_ip"`
    PlatformArgs    map[string]string `mapstructure:"platform_args"`

    RawSSHWaitTimeout string    `mapstructure:"ssh_wait_timeout"`

    SSHPassword     string      `mapstructure:"ssh_password"`
    SSHUser         string      `mapstructure:"ssh_username"`
    SSHKeyPath      string      `mapstructure:"ssh_key_path"`

    OutputDir       string      `mapstructure:"output_directory"`

    tpl *packer.ConfigTemplate
}


type Builder struct {
    config config
    runner multistep.Runner
}


func (self *Builder) Prepare (raws ...interface{}) (params []string, retErr error) {

    md, err := common.DecodeConfig(&self.config, raws...)
    if err != nil {
        return nil, err
    }

    errs := common.CheckUnusedConfig(md)
    if errs == nil {
        errs = &packer.MultiError{}
    }

    self.config.tpl, err = packer.NewConfigTemplate()

    if err != nil {
        return nil, err
    }

    // Set default vaules

    if self.config.VncPortMin == 0 {
        self.config.VncPortMin = 5900
    }

    if self.config.VncPortMax == 0 {
        self.config.VncPortMax = 6000
    }

    if self.config.RawBootWait == "" {
        self.config.RawBootWait = "5s"
    }

    if self.config.HTTPPortMin == 0 {
        self.config.HTTPPortMin = 8000
    }

    if self.config.HTTPPortMax == 0 {
        self.config.HTTPPortMax = 9000
    }

    if self.config.RawSSHWaitTimeout == "" {
        self.config.RawSSHWaitTimeout = "200m"
    }

    if self.config.OutputDir == "" {
        self.config.OutputDir = fmt.Sprintf("output-%s", self.config.PackerBuildName)
    }

    templates := map[string]*string {
        "username":             &self.config.Username,
        "password":             &self.config.Password,
        "host_ip":              &self.config.HostIp,
        "iso_url":              &self.config.IsoUrl,
        "instance_name":        &self.config.InstanceName,
        "root_disk_size":       &self.config.RootDiskSize,
        "clone_template":       &self.config.CloneTemplate,
        "iso_uuid":             &self.config.IsoUuid,
        "sr_uuid":              &self.config.SrUuid,
        "network_uuid":         &self.config.NetworkUuid,
        "boot_wait":            &self.config.RawBootWait,
        "iso_checksum":         &self.config.ISOChecksum,
        "iso_checksum_type":    &self.config.ISOChecksumType,
        "http_directory":       &self.config.HTTPDir,
        "local_ip":             &self.config.LocalIp,
        "ssh_wait_timeout":     &self.config.RawSSHWaitTimeout,
        "ssh_username":         &self.config.SSHUser,
        "ssh_password":         &self.config.SSHPassword,
        "ssh_key_path":         &self.config.SSHKeyPath,
        "output_directory":     &self.config.OutputDir,
    }


    for n, ptr := range templates {
        var err error
        *ptr, err = self.config.tpl.Process(*ptr, nil)
        if err != nil {
            errs = packer.MultiErrorAppend(errs, fmt.Errorf("Error processing %s: %s", n, err))
        }
    }

/*
    if self.config.IsoUrl == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("a iso url must be specified"))
    }
*/

    self.config.BootWait, err = time.ParseDuration(self.config.RawBootWait)
    if err != nil {
        errs = packer.MultiErrorAppend(
                errs, errors.New("Failed to parse boot_wait."))
    }

    self.config.sshWaitTimeout, err = time.ParseDuration(self.config.RawSSHWaitTimeout)
    if err != nil {
        errs = packer.MultiErrorAppend(
                errs, fmt.Errorf("Failed to parse ssh_wait_timeout: %s", err))
    }

    for i, command := range self.config.BootCommand {
        if err := self.config.tpl.Validate(command); err != nil {
            errs = packer.MultiErrorAppend(errs,
                    fmt.Errorf("Error processing boot_command[%d]: %s", i, err))
        }
    }

    if self.config.SSHUser == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("An ssh_username must be specified."))
    }

    if self.config.SSHKeyPath != "" {
        if _, err := os.Stat(self.config.SSHKeyPath); err != nil {
            errs = packer.MultiErrorAppend(
                    errs, fmt.Errorf("ssh_key_path is invalid: %s", err))
        } else if _, err := commonssh.FileSigner(self.config.SSHKeyPath); err != nil {
            errs = packer.MultiErrorAppend(
                    errs, fmt.Errorf("ssh_key_path is invalid: %s", err))
        }
    }

    if self.config.Username == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("A username for the xenserver host must be specified."))
    }

    if self.config.Password == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("A password for the xenserver host must be specified."))
    }

    if self.config.HostIp == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("An ip for the xenserver host must be specified."))
    }

    if self.config.InstanceName == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("An insatnce name must be specified."))
    }

    if self.config.RootDiskSize == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("A root disk size must be specified."))
    }

    if self.config.CloneTemplate == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("A template to clone from must be specified."))
    }

    if self.config.IsoUuid == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("a uuid for the installation iso must be specified."))
    }

    if self.config.SrUuid == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("a uuid for the sr used for the instance must be specified."))
    }

    if self.config.NetworkUuid == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("a uuid for the network used for the instance must be specified."))
    }

    if self.config.LocalIp == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("A local IP visible to XenServer's mangement interface is required to serve files."))
    }

    if len(self.config.PlatformArgs) == 0 {
        pargs := make(map[string]string)
        pargs["viridian"] = "false"
        pargs["nx"] = "true"
        pargs["pae"] = "true"
        pargs["apic"] = "true"
        pargs["timeoffset"] = "0"
        pargs["acpi"] = "1"
        self.config.PlatformArgs = pargs
    }

    if self.config.HTTPPortMin > self.config.HTTPPortMax {
        errs = packer.MultiErrorAppend(
                errs, errors.New("the HTTP min port must be less than the max"))
    }

    if self.config.VncPortMin > self.config.VncPortMax {
        errs = packer.MultiErrorAppend(
                errs, errors.New("the VNC min port must be less than the max"))
    }

    if self.config.ISOChecksumType == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("The iso_checksum_type must be specified."))
    } else {
        self.config.ISOChecksumType = strings.ToLower(self.config.ISOChecksumType)
        if self.config.ISOChecksumType != "none" {
            if self.config.ISOChecksum == "" {
                errs = packer.MultiErrorAppend(
                        errs, errors.New("Due to the file size being large, an iso_checksum is required."))
            } else {
                self.config.ISOChecksum = strings.ToLower(self.config.ISOChecksum)
            }

            if hash := common.HashForType(self.config.ISOChecksumType); hash == nil {
                errs = packer.MultiErrorAppend(
                        errs, fmt.Errorf("Unsupported checksum type: %s", self.config.ISOChecksumType))
            }

        }
    }

    if self.config.ISOUrl == "" {
        errs = packer.MultiErrorAppend(
                errs, errors.New("A ISO URL must be specfied."))
    } else {
        self.config.ISOUrls = []string{self.config.ISOUrl}
    }

    for i, url := range self.config.ISOUrls {
        self.config.ISOUrls[i], err = common.DownloadableURL(url)
        if err != nil {
            errs = packer.MultiErrorAppend(
                    errs, fmt.Errorf("Failed to parse the iso_url (%d): %s", i, err))
        }
    }

    if len(errs.Errors) > 0 {
        retErr = errors.New(errs.Error())
    }

    return nil, retErr

}

func (self *Builder) Run(ui packer.Ui, hook packer.Hook, cache packer.Cache) (packer.Artifact, error) {
    //Setup XAPI client
    client := NewXenAPIClient(self.config.HostIp, self.config.Username, self.config.Password)

    err := client.Login()
    if err != nil {
        return nil, err.(error)
    }
    ui.Say("XAPI client session established")

    client.GetHosts()

    //Share state between the other steps using a statebag
    state := new(multistep.BasicStateBag)
    state.Put("cache", cache)
    state.Put("client", client)
    state.Put("config", self.config)
    state.Put("hook", hook)
    state.Put("ui", ui)


    //Build the steps
    steps := []multistep.Step{
        &common.StepDownload{
            Checksum:       self.config.ISOChecksum,
            ChecksumType:   self.config.ISOChecksumType,
            Description:    "ISO",
            ResultKey:      "iso_path",
            Url:            self.config.ISOUrls,
        },
        new(stepPrepareOutputDir),
        new(stepHTTPServer),
        new(stepUploadIso),
        new(stepCreateInstance),
        new(stepStartVmPaused),
        new(stepForwardVncPortOverSsh),
        new(stepBootWait),
        new(stepTypeBootCommand),
        new(stepWait),
        &common.StepConnectSSH{
            SSHAddress: sshAddress,
            SSHConfig:  sshConfig,
            SSHWaitTimeout: self.config.sshWaitTimeout,
        },
        new(common.StepProvision),
        new(stepShutdownAndExport),
    }

    self.runner = &multistep.BasicRunner{Steps: steps}
    self.runner.Run(state)

    artifact, _ := NewArtifact(self.config.OutputDir)

    if rawErr, ok := state.GetOk("error"); ok {
        return nil, rawErr.(error)
    }

    return artifact, nil
}


func (self *Builder) Cancel() {
    if self.runner != nil {
        log.Println("Cancelling the step runner...")
        self.runner.Cancel()
    }
    fmt.Println("Cancelling the builder")
}
