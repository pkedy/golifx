package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/pdf/golifx/common"
	"github.com/spf13/cobra"
)

var (
	flagLightIDs        []int
	flagLightLabels     []string
	flagLightHue        uint16
	flagLightSaturation uint16
	flagLightBrightness uint16
	flagLightKelvin     uint16
	flagLightDuration   time.Duration

	cmdLightList = &cobra.Command{
		Use:     `list`,
		Short:   `list available lights`,
		PreRun:  setupClient,
		Run:     lightList,
		PostRun: closeClient,
	}

	cmdLightColor = &cobra.Command{
		Use:     `color`,
		Short:   `set light color`,
		PreRun:  setupClient,
		Run:     lightColor,
		PostRun: closeClient,
	}

	cmdLightPower = &cobra.Command{
		Use:       `power`,
		Short:     `[on|off]`,
		Long:      `[on|off]`,
		ValidArgs: []string{`on`, `off`},
		PreRun:    setupClient,
		Run:       lightPower,
		PostRun:   closeClient,
	}

	cmdLight = &cobra.Command{
		Use:   `light`,
		Short: `interact with lights`,
		Long: `Interact with lights.
Acts on all lights by default, however you may restrict the lights that a command applies to by specifying IDs or labels via the flags listed below.`,
		Run: usage,
	}
)

func init() {
	cmdLightColor.Flags().Uint16VarP(&flagLightHue, `hue`, `H`, 0, `hue component of the HSBK color (0-65535)`)
	cmdLightColor.Flags().Uint16VarP(&flagLightSaturation, `saturation`, `S`, 0, `saturation component of the HSBK color (0-65535)`)
	cmdLightColor.Flags().Uint16VarP(&flagLightBrightness, `brightness`, `B`, 0, `brightness component of the HSBK color (0-65535)`)
	cmdLightColor.Flags().Uint16VarP(&flagLightKelvin, `kelvin`, `K`, 0, `kelvin component of the HSBK color, the color temperature of whites (2500-9000)`)
	cmdLightColor.Flags().DurationVarP(&flagLightDuration, `duration`, `d`, 0*time.Second, `duration of the color transition`)
	cmdLightColor.MarkFlagRequired(`hue`)
	cmdLightColor.MarkFlagRequired(`saturation`)
	cmdLightColor.MarkFlagRequired(`brightness`)
	cmdLightColor.MarkFlagRequired(`kelvin`)
	cmdLightColor.MarkFlagRequired(`duration`)
	cmdLight.AddCommand(cmdLightList)
	cmdLight.AddCommand(cmdLightColor)
	cmdLight.AddCommand(cmdLightPower)

	cmdLight.PersistentFlags().IntSliceVarP(&flagLightIDs, `id`, `i`, make([]int, 0), `ID of the light(s) to manage, comma-seprated.  Defaults to all lights`)
	cmdLight.PersistentFlags().StringSliceVarP(&flagLightLabels, `label`, `l`, make([]string, 0), `label of the light(s) to manage, comma-separated.  Defaults to all lights.`)
}

func lightList(c *cobra.Command, args []string) {
	var (
		err    error
		lights []common.Light
	)

	timeout := time.After(flagTimeout)
	tick := time.Tick(100 * time.Millisecond)
	timedOut := false

	for {
		select {
		case <-tick:
			lights, err = client.GetLights()
			if err != nil && err != common.ErrNotFound {
				logger.WithField(`error`, err).Fatalln(`Could not find lights`)
			}
		case <-timeout:
			if len(lights) == 0 {
				logger.Fatalln(`No lights found`)
			}
			timedOut = true
			break
		}
		if timedOut {
			break
		}
	}

	table := new(tabwriter.Writer)
	table.Init(os.Stdout, 0, 4, 4, ' ', 0)
	fmt.Fprintf(table, fmt.Sprintf("%s\t%s\t%s\t%s\n", `ID`, `Label`, `Power`, `Color`))

	for _, l := range lights {
		label, err := l.GetLabel()
		if err != nil {
			logger.WithField(`light_id`, l.ID()).Warnln(`Couldn't get color for light`)
			continue
		}
		power, err := l.GetPower()
		if err != nil {
			logger.WithField(`light_id`, l.ID()).Warnln(`Couldn't get color for light`)
			continue
		}
		color, err := l.GetColor()
		if err != nil {
			logger.WithField(`light_id`, l.ID()).Warnln(`Couldn't get color for light`)
			continue
		}
		fmt.Fprintf(table, "%v\t%s\t%v\t%+v\n", l.ID(), label, power, color)
	}
	fmt.Fprintln(table)
	table.Flush()
}

func getLights() []common.Light {
	var lights []common.Light

	logger.WithField(`ids`, flagLightLabels).Debug(`Requested IDs`)
	logger.WithField(`labels`, flagLightLabels).Debug(`Requested labels`)

	if len(flagLightIDs) > 0 {
		for _, id := range flagLightIDs {
			light, err := client.GetLightByID(uint64(id))
			if err != nil {
				logger.WithField(`error`, err).Fatalf("Could not find light with ID '%v': %v", id, err)
			}
			lights = append(lights, light)
		}
	}
	if len(flagLightLabels) > 0 {
		for _, label := range flagLightLabels {
			light, err := client.GetLightByLabel(label)
			if err != nil {
				logger.WithField(`error`, err).Fatalf("Could not find light with label '%v': %v", label, err)
			}
			lights = append(lights, light)
		}
	}

	return lights
}

func lightPower(c *cobra.Command, args []string) {
	if len(args) < 1 {
		c.Usage()
		logger.Fatalln(`Missing state (on|off)`)
	}

	var state bool

	switch args[0] {
	case `on`:
		state = true
	case `off`:
		state = false
	default:
		c.Usage()
		logger.WithField(`state`, args[0]).Fatalln(`Invalid power state requested`)
	}

	lights := getLights()

	if len(lights) > 0 {
		for _, light := range lights {
			light.SetPower(state)
		}
	} else {
		client.SetPower(state)
	}
}

func lightColor(c *cobra.Command, args []string) {
	if flagLightHue == 0 && flagLightSaturation == 0 && flagLightBrightness == 0 && flagLightKelvin == 0 {
		c.Usage()
		logger.Fatalln(`Missing color definition`)
	}

	lights := getLights()

	color := common.Color{
		Hue:        flagLightHue,
		Saturation: flagLightSaturation,
		Brightness: flagLightBrightness,
		Kelvin:     flagLightKelvin,
	}

	if len(lights) > 0 {
		for _, light := range lights {
			light.SetColor(color, flagLightDuration)
		}
	} else {
		client.SetColor(color, flagLightDuration)
	}
}