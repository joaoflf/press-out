document.addEventListener("DOMContentLoaded", function () {
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
