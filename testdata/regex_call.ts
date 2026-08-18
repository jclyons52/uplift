const re = /a+/g;
function has(s: string): boolean {
    return re.test(s);
}
function find(s: string): string {
    return re.exec(s);
}
