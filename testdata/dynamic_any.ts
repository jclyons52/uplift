function format(results: any): string {
    let out = "";
    for (const result of results) {
        const line = result.line || 0;
        const severity = result.severity;
        const n = result.messages.length;
        out += result.filePath + ":" + n + ":" + severity + ":" + line;
    }
    results.forEach((r: any) => {
        out += r.name;
    });
    return out;
}
