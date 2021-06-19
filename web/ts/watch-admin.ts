function usePreset(cID: number, lectureHallID: number, presetID: number, streamID: number) {
    (document.getElementById("presetImage" + lectureHallID + "-" + presetID) as HTMLImageElement).classList.add("animate-pulse")
    postData("/api/course/" + cID + "/switchPreset/" + lectureHallID + "/" + presetID + "/" + (document.getElementById("streamID") as HTMLInputElement).value).then(
        function (res) {
            (document.getElementById("presetImage" + lectureHallID + "-" + presetID) as HTMLImageElement).classList.remove("animate-pulse")
        }
    )
}

function moderatedChange(evt: HTMLInputElement) {
    postData("/api/stream/chatMode/" + (document.getElementById("streamID") as HTMLInputElement).value,
        {"moderated": evt.checked}
    ).then(r => {
        if (r.status !== 200) {
            console.log(r.status)
        }
    });
}