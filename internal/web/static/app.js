// Small progressive enhancements for the Snip dashboard. Everything works without JavaScript
// except the copy buttons.

document.addEventListener('click', async (event) => {
  const button = event.target.closest('button[data-copy]');
  if (!button) return;

  const original = button.textContent;
  try {
    await navigator.clipboard.writeText(button.dataset.copy);
    button.textContent = 'Copied!';
  } catch {
    button.textContent = 'Press Ctrl+C';
    window.prompt('Copy this link:', button.dataset.copy);
  }
  setTimeout(() => {
    button.textContent = original;
  }, 1500);
});

// Forms with data-confirm ask before submitting (delete, reset stats).
document.addEventListener('submit', (event) => {
  const form = event.target.closest('form[data-confirm]');
  if (!form) return;
  if (!window.confirm(form.dataset.confirm)) {
    return;
  }
});

// Hide the flash message after a few seconds.
const flash = document.querySelector('.flash');
if (flash) {
  setTimeout(() => flash.classList.add('flash-hidden'), 6000);
}
