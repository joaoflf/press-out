document.addEventListener("DOMContentLoaded", function () {
  var fileInput = document.getElementById("upload-file");
  var submitBtn = document.getElementById("upload-submit");
  var form = document.getElementById("upload-form");

  if (!fileInput || !submitBtn || !form) return;

  var validExts = [".mp4", ".mov"];
  var validMimes = ["video/mp4", "video/quicktime"];

  // Pose estimation state
  var keypointsBlob = null;
  var poseProcessing = false;

  function getRadioValue() {
    var radios = form.querySelectorAll('input[name="lift_type"]');
    for (var i = 0; i < radios.length; i++) {
      if (radios[i].checked) return radios[i].value;
    }
    return "";
  }

  function isValidFile() {
    if (!fileInput.files || fileInput.files.length === 0) return false;
    var file = fileInput.files[0];
    var ext = file.name.substring(file.name.lastIndexOf(".")).toLowerCase();
    if (validExts.indexOf(ext) !== -1) return true;
    if (validMimes.indexOf(file.type) !== -1) return true;
    return false;
  }

  function updateSubmit() {
    var hasFile = isValidFile();
    var hasType = getRadioValue() !== "";
    submitBtn.disabled = !(hasFile && hasType && !poseProcessing);
  }

  // --- Pose estimation ---

  var SMOOTH_WINDOW = 3; // frames on each side (total window = 7)

  function seekTo(video, time) {
    return new Promise(function (resolve) {
      if (Math.abs(video.currentTime - time) < 0.01) {
        resolve();
        return;
      }
      var handler = function () {
        video.removeEventListener("seeked", handler);
        resolve();
      };
      video.addEventListener("seeked", handler);
      video.currentTime = time;
    });
  }

  function sleep(ms) {
    return new Promise(function (resolve) { setTimeout(resolve, ms); });
  }

  function detectOnce(bodyPose, source) {
    return new Promise(function (resolve, reject) {
      try {
        bodyPose.detect(source, function (results) {
          resolve(results || []);
        });
      } catch (e) {
        reject(e);
      }
    });
  }

  function normBox(box, w, h) {
    if (!box) return { left: 0, top: 0, right: 1, bottom: 1 };
    return {
      left: (box.xMin || 0) / w,
      top: (box.yMin || 0) / h,
      right: (box.xMax || w) / w,
      bottom: (box.yMax || h) / h
    };
  }

  function smoothFrames(allFrames) {
    var smoothed = [];
    for (var ci = 0; ci < allFrames.length; ci++) {
      var center = allFrames[ci];
      if (center.keypoints.length === 0) {
        smoothed.push(center);
        continue;
      }

      var numKps = center.keypoints.length;
      var sums = [];
      var counts = [];
      for (var k = 0; k < numKps; k++) {
        sums.push({ x: 0, y: 0, confidence: 0 });
        counts.push(0);
      }

      var lo = Math.max(0, ci - SMOOTH_WINDOW);
      var hi = Math.min(allFrames.length - 1, ci + SMOOTH_WINDOW);

      for (var i = lo; i <= hi; i++) {
        var f = allFrames[i];
        if (f.keypoints.length !== numKps) continue;
        for (var k2 = 0; k2 < numKps; k2++) {
          var kp = f.keypoints[k2];
          if (kp.confidence > 0.15) {
            sums[k2].x += kp.x;
            sums[k2].y += kp.y;
            sums[k2].confidence += kp.confidence;
            counts[k2]++;
          }
        }
      }

      var smoothedKps = center.keypoints.map(function (kp, k3) {
        if (counts[k3] === 0) return kp;
        return {
          name: kp.name,
          x: sums[k3].x / counts[k3],
          y: sums[k3].y / counts[k3],
          confidence: sums[k3].confidence / counts[k3]
        };
      });

      smoothed.push({
        timeOffsetMs: center.timeOffsetMs,
        boundingBox: center.boundingBox,
        keypoints: smoothedKps
      });
    }
    return smoothed;
  }

  async function runPoseEstimation(videoFile) {
    var progressEl = document.getElementById("pose-progress");
    var statusEl = document.getElementById("pose-status");
    var counterEl = document.getElementById("pose-frame-counter");
    var barEl = document.getElementById("pose-progress-bar");

    if (!progressEl || typeof ml5 === "undefined") {
      console.warn("ml5.js not available, skipping pose estimation");
      return null;
    }

    poseProcessing = true;
    updateSubmit();
    progressEl.classList.remove("hidden");
    statusEl.textContent = "Loading pose model...";
    counterEl.textContent = "";
    barEl.value = 0;

    var video = document.createElement("video");
    video.muted = true;
    video.playsInline = true;
    video.src = URL.createObjectURL(videoFile);

    await new Promise(function (resolve) {
      video.onloadedmetadata = resolve;
    });

    var offscreen = document.createElement("canvas");
    offscreen.width = video.videoWidth;
    offscreen.height = video.videoHeight;
    var offCtx = offscreen.getContext("2d");

    var bodyPose;
    try {
      bodyPose = await new Promise(function (resolve, reject) {
        try {
          var m = ml5.bodyPose("MoveNet", { modelType: "SINGLEPOSE_THUNDER" }, function () {
            resolve(m);
          });
        } catch (e) {
          reject(e);
        }
      });
    } catch (e) {
      console.error("Failed to load pose model:", e);
      progressEl.classList.add("hidden");
      poseProcessing = false;
      updateSubmit();
      return null;
    }

    statusEl.textContent = "Detecting poses...";

    var fps = 30;
    var step = 1 / fps;
    var duration = video.duration;
    var totalFrames = Math.floor(duration * fps);
    var allFrames = [];
    var frameIdx = 0;

    for (var t = 0; t < duration; t += step) {
      await seekTo(video, t);
      await sleep(30);
      offCtx.drawImage(video, 0, 0, offscreen.width, offscreen.height);

      var results;
      try {
        results = await detectOnce(bodyPose, offscreen);
      } catch (e) {
        results = [];
      }

      var pose = results.length > 0 ? results[0] : null;

      allFrames.push({
        timeOffsetMs: Math.round(t * 1000),
        boundingBox: pose && pose.box ? normBox(pose.box, video.videoWidth, video.videoHeight) : null,
        keypoints: pose ? pose.keypoints.map(function (kp) {
          return {
            name: kp.name,
            x: kp.x / video.videoWidth,
            y: kp.y / video.videoHeight,
            confidence: kp.confidence
          };
        }) : []
      });

      frameIdx++;
      var pct = Math.round((frameIdx / totalFrames) * 100);
      barEl.value = pct;
      counterEl.textContent = "Frame " + frameIdx + " / " + totalFrames;
    }

    URL.revokeObjectURL(video.src);

    // Apply 7-frame smoothing
    var smoothedFrames = smoothFrames(allFrames);

    var result = {
      sourceWidth: video.videoWidth,
      sourceHeight: video.videoHeight,
      frames: smoothedFrames
    };

    statusEl.textContent = "Pose detection complete";
    poseProcessing = false;
    updateSubmit();

    var json = JSON.stringify(result);
    return new Blob([json], { type: "application/json" });
  }

  // --- Event handlers ---

  fileInput.addEventListener("change", function () {
    if (fileInput.files && fileInput.files.length > 0 && !isValidFile()) {
      alert("Please select an MP4 or MOV video file.");
      fileInput.value = "";
      updateSubmit();
      return;
    }

    keypointsBlob = null;

    if (isValidFile()) {
      runPoseEstimation(fileInput.files[0]).then(function (blob) {
        keypointsBlob = blob;
      }).catch(function (err) {
        console.error("Pose estimation error:", err);
        keypointsBlob = null;
      });
    }

    updateSubmit();
  });

  var radios = form.querySelectorAll('input[name="lift_type"]');
  for (var i = 0; i < radios.length; i++) {
    radios[i].addEventListener("change", updateSubmit);
  }

  // Intercept form submit to append keypoints as multipart field.
  form.addEventListener("submit", function (e) {
    if (!keypointsBlob) return; // let the form submit normally

    e.preventDefault();

    var formData = new FormData(form);
    formData.append("keypoints", keypointsBlob, "keypoints.json");

    fetch(form.action, {
      method: "POST",
      body: formData
    }).then(function (resp) {
      if (resp.redirected) {
        window.location.href = resp.url;
      } else if (resp.ok) {
        window.location.href = "/";
      }
    }).catch(function (err) {
      console.error("Upload failed:", err);
    });
  });

  var modal = document.getElementById("upload-modal");
  if (modal) {
    modal.addEventListener("close", function () {
      form.reset();
      submitBtn.disabled = true;
      keypointsBlob = null;
      poseProcessing = false;
      var progressEl = document.getElementById("pose-progress");
      if (progressEl) progressEl.classList.add("hidden");
    });
  }
});
