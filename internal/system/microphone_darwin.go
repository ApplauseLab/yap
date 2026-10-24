//go:build darwin

package system

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AVFoundation -framework Foundation

#import <AVFoundation/AVFoundation.h>
#import <dispatch/dispatch.h>

// Returns: 0 = undetermined, 1 = denied/restricted, 2 = granted
static int microphonePermissionStatus(void) {
    if (@available(macOS 10.14, *)) {
        AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
        switch (status) {
            case AVAuthorizationStatusNotDetermined:
                return 0;
            case AVAuthorizationStatusRestricted:
            case AVAuthorizationStatusDenied:
                return 1;
            case AVAuthorizationStatusAuthorized:
                return 2;
            default:
                return 0;
        }
    }
    return 2;
}

// Requests microphone permission and returns latest status.
// Returns: 0 = undetermined, 1 = denied/restricted, 2 = granted
static int requestMicrophonePermission(void) {
    if (@available(macOS 10.14, *)) {
        __block BOOL granted = NO;
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);

        [AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL allowed) {
            granted = allowed;
            dispatch_semaphore_signal(sem);
        }];

        dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);

        if (granted) {
            return 2;
        }
        return microphonePermissionStatus();
    }
    return 2;
}
*/
import "C"

func CheckMicrophonePermission() string {
	switch int(C.microphonePermissionStatus()) {
	case 2:
		return "granted"
	case 1:
		return "denied"
	default:
		return "undetermined"
	}
}

func RequestMicrophonePermission() string {
	switch int(C.requestMicrophonePermission()) {
	case 2:
		return "granted"
	case 1:
		return "denied"
	default:
		return "undetermined"
	}
}
