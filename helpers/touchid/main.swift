import Foundation
import LocalAuthentication

// Invoked by agent-pwd when a high-risk action needs confirmation.
// Arguments: <random-nonce> <reason>. Exit 0 means the user verified.
// Any failure is expected to fall back to terminal master-password entry.
let args = CommandLine.arguments
guard args.count >= 2 else { exit(2) }

let context = LAContext()
context.localizedFallbackTitle = "Use master password"
var policyError: NSError?
guard context.canEvaluatePolicy(.deviceOwnerAuthentication, error: &policyError) else {
    exit(3)
}

let reason = args.count >= 3 ? args[2] : "Confirm a high-risk agent-pwd action"
let semaphore = DispatchSemaphore(value: 0)
var verified = false
context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason) { ok, _ in
    verified = ok
    semaphore.signal()
}
semaphore.wait()
exit(verified ? 0 : 4)
