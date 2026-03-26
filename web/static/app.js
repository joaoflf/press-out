document.addEventListener("DOMContentLoaded", function () {
  // --- Video Player: Toggle & Speed Controls ---
  var video = document.getElementById("lift-video");
  if (video) {
    var overlay = document.getElementById("video-toggle-overlay");
    var badge = document.getElementById("mode-badge");
    var speedBtns = document.querySelectorAll(".speed-btn");

    // Toggle handler (skeleton <-> clean)
    if (overlay && video.dataset.skeletonSrc && video.dataset.cleanSrc) {
      var isSkeleton = true;
      overlay.addEventListener("click", function (e) {
        e.preventDefault();
        e.stopPropagation();

        var time = video.currentTime;
        var rate = video.playbackRate;
        var wasPaused = video.paused;

        video.src = isSkeleton ? video.dataset.cleanSrc : video.dataset.skeletonSrc;
        isSkeleton = !isSkeleton;

        video.addEventListener("loadedmetadata", function () {
          video.currentTime = time;
          video.playbackRate = rate;
          if (!wasPaused) video.play();
        }, {once: true});

        video.load();

        if (badge) {
          badge.textContent = isSkeleton ? "Skeleton" : "Clean";
        }
      });
    }

    // Speed handler
    for (var i = 0; i < speedBtns.length; i++) {
      speedBtns[i].addEventListener("click", function () {
        var speed = parseFloat(this.dataset.speed);
        video.playbackRate = speed;

        for (var j = 0; j < speedBtns.length; j++) {
          speedBtns[j].classList.remove("text-[#8BA888]");
          speedBtns[j].classList.add("text-white/80");
        }
        this.classList.remove("text-white/80");
        this.classList.add("text-[#8BA888]");
      });
    }
  }

  // --- Upload Form ---
  var fileInput = document.getElementById("upload-file");
  var submitBtn = document.getElementById("upload-submit");
  var form = document.getElementById("upload-form");

  if (!fileInput || !submitBtn || !form) return;

  var validExts = [".mp4", ".mov"];
  var validMimes = ["video/mp4", "video/quicktime"];

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
    submitBtn.disabled = !(hasFile && hasType);
  }

  fileInput.addEventListener("change", function () {
    if (fileInput.files && fileInput.files.length > 0 && !isValidFile()) {
      alert("Please select an MP4 or MOV video file.");
      fileInput.value = "";
      updateSubmit();
      return;
    }
    updateSubmit();
  });

  var radios = form.querySelectorAll('input[name="lift_type"]');
  for (var i = 0; i < radios.length; i++) {
    radios[i].addEventListener("change", updateSubmit);
  }

  var modal = document.getElementById("upload-modal");
  if (modal) {
    modal.addEventListener("close", function () {
      form.reset();
      submitBtn.disabled = true;
    });
  }
});
