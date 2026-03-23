package sys

import (
	"fmt"
	"os"
)

const ledTriggerPath = "/sys/class/leds/led0/trigger"

// SetLEDTrigger sets the trigger mode for the onboard LED (led0). Common
// triggers include "timer", "heartbeat", "default-on", and "none".
func SetLEDTrigger(trigger string) error {
	if err := os.WriteFile(ledTriggerPath, []byte(trigger), 0644); err != nil {
		return fmt.Errorf("set LED trigger to %s: %w", trigger, err)
	}
	return nil
}

// setLEDDelayMS sets the on and off delay for the timer trigger in
// milliseconds.
func setLEDDelayMS(onMS, offMS int) error {
	const delayOnPath = "/sys/class/leds/led0/delay_on"
	const delayOffPath = "/sys/class/leds/led0/delay_off"

	if err := os.WriteFile(delayOnPath, []byte(fmt.Sprintf("%d", onMS)), 0644); err != nil {
		return fmt.Errorf("set LED delay_on: %w", err)
	}
	if err := os.WriteFile(delayOffPath, []byte(fmt.Sprintf("%d", offMS)), 0644); err != nil {
		return fmt.Errorf("set LED delay_off: %w", err)
	}
	return nil
}

// SlowBlink sets the LED to blink slowly (1 second on, 1 second off),
// indicating normal idle operation.
func SlowBlink() error {
	if err := SetLEDTrigger("timer"); err != nil {
		return err
	}
	return setLEDDelayMS(1000, 1000)
}

// FastBlink sets the LED to blink rapidly (200ms on, 200ms off),
// indicating active archiving or busy state.
func FastBlink() error {
	if err := SetLEDTrigger("timer"); err != nil {
		return err
	}
	return setLEDDelayMS(200, 200)
}

// DoubleBlink sets the LED to a double-blink pattern (short on, short off,
// short on, long off) by using a heartbeat trigger, indicating a warning
// condition.
func DoubleBlink() error {
	return SetLEDTrigger("heartbeat")
}

// LEDOff turns the LED off completely.
func LEDOff() error {
	return SetLEDTrigger("none")
}
