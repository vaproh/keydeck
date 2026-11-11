(() => {
  const toast = document.getElementById('toast');
  let toastTimer;

  function notify(message, tone = 'ok') {
    if (!toast) return;
    toast.textContent = message;
    toast.dataset.tone = tone;
    toast.classList.add('is-visible');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => toast.classList.remove('is-visible'), 3200);
  }

  window.deskNotify = notify;

  window.presetIssue = ({ presets, productId }) => ({
    presets,
    productId: String(productId || ''),
    presetId: '',
    get presetsForProduct() {
      return this.presets.filter((preset) => String(preset.product_id) === String(this.productId));
    },
    syncAction() {
      this.$el.action = '/admin/products/' + this.productId + '/licenses';
      if (!this.presetsForProduct.some((preset) => String(preset.id) === String(this.presetId))) {
        this.presetId = '';
      }
    },
    applyPreset() {
      const preset = this.presetsForProduct.find((item) => String(item.id) === String(this.presetId));
      if (!preset) return;
      this.$refs.planName.value = preset.name;
      this.$refs.keyType.value = preset.key_type;
      this.$refs.days.value = preset.validity_days || 30;
      this.$refs.trialMinutes.value = preset.trial_minutes || 15;
      this.$refs.maxActivations.value = preset.max_activations || 1;
      this.$refs.graceSeconds.value = preset.grace_seconds || 0;
      this.$refs.limits.value = preset.limits_json;
      this.$refs.features.value = preset.features_json;
      this.$refs.starts.checked = !!preset.starts_on_activation;
      this.$refs.oneTime.checked = !!preset.one_time_activation;
    },
  });

  window.deskCopy = async (text) => {
    try {
      await navigator.clipboard.writeText(text);
      notify('Key copied to clipboard.');
    } catch {
      notify('Copy failed. Select the key manually.', 'bad');
    }
  };

  document.addEventListener('htmx:beforeRequest', () => {
    document.documentElement.classList.add('is-loading');
  });
  document.addEventListener('htmx:afterRequest', () => {
    document.documentElement.classList.remove('is-loading');
  });
  document.addEventListener('htmx:responseError', (event) => {
    notify('Request failed (' + event.detail.xhr.status + ').', 'bad');
  });
  document.addEventListener('htmx:afterSwap', () => {
    document.querySelectorAll('[data-stagger]').forEach((el) => {
      el.style.animationDelay = '0ms';
    });
  });
})();
