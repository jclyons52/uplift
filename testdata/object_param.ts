/** @param {Object} message */
function level(message: Object): number {
    if (message.fatal || message.severity === 2) {
        return 2;
    }
    return 1;
}
