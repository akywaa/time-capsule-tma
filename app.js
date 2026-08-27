/*
 * TODO: потом мб перепишу эту чушь на реакт или vue
 * пока лень настраивать сборку, и так сойдет 
 * На ваниле бандл меньше и грузится моментально, для мвп пойдет. 
 */
// const DEBUG = true;
// Init TWA
const tg = window.Telegram.WebApp;
tg.ready();
tg.expand();
tg.setHeaderColor('#03050a');
tg.setBackgroundColor('#03050a');

const BOT_USERNAME = "KapsulaVremeni_bot";
const startParam = tg.initDataUnsafe?.start_param;

// State
let currentCapsuleId = null;
let currentMediaType = 'text';
let currentCapsuleType = 'personal';
let currentModelType = 'safe';
let selectedHours = 24;
let photoBase64 = null;
let voiceBase64 = null;
let audioBlob = null;
let mediaRecorder = null;
let voiceStartTime = 0;
let voiceTimerInterval = null;
let timerInterval;
let geoWatchId = null;
let lastGeoPos = null;
let geoMap = null;
let geoMarker = null;
let userMarker = null;
let geoSearchTimeout = null;
let capsuleData = null;
let currentStep = 1;

if (startParam) {
    currentCapsuleId = startParam;
    document.getElementById('view-view').classList.remove('hidden');
    loadCapsule(currentCapsuleId);
} else {
    document.getElementById('home-view').classList.remove('hidden');
    loadMyCapsules();
}

function startCreationFlow() {
    document.getElementById('home-view').classList.add('hidden');
    document.getElementById('create-view').classList.remove('hidden');
    showStep(1);
    tg.BackButton.show();
    tg.BackButton.onClick(handleTgBack);
    tg.HapticFeedback.impactOccurred('light');
}

function cancelCreation() {
    document.getElementById('create-view').classList.add('hidden');
    document.getElementById('home-view').classList.remove('hidden');
    tg.BackButton.hide();
    tg.BackButton.offClick(handleTgBack);
}

function handleTgBack() {
    if (currentStep > 1) {
        prevStep(currentStep - 1);
    } else {
        cancelCreation();
    }
}

function showStep(step) {
    document.querySelectorAll('.create-step').forEach(el => el.classList.remove('active'));
    document.getElementById(`step-${step}`).classList.add('active');

    const dots = document.querySelectorAll('.step-dot');
    dots.forEach((dot, index) => {
        dot.classList.toggle('active', index === (step - 1));
    });

    currentStep = step;
    tg.HapticFeedback.selectionChanged();
}

function nextStep(targetStep) {
    if (currentStep === 2) {
        if (currentMediaType === 'text' && !document.getElementById('text-input').value.trim()) {
            return tg.showAlert('Напиши секрет перед тем как идти дальше!');
        }
        if (currentMediaType === 'photo' && !photoBase64) {
            return tg.showAlert('Прикрепи фото!');
        }
        if (currentMediaType === 'voice' && !voiceBase64) {
            return tg.showAlert('Запиши голосовое!');
        }
    }
    if (currentStep === 3 && currentCapsuleType === 'geo') {
        const lat = parseFloat(document.getElementById('geo-lat').value);
        if (!lat || lat === 0) {
            return tg.showAlert('Сначала определи местоположение!');
        }
    }
    showStep(targetStep);
}

function prevStep(targetStep) {
    showStep(targetStep);
}

async function loadMyCapsules() {
    try {
        const hdrs = {};
        if (tg.initData) {
            hdrs['X-Telegram-Init-Data'] = tg.initData;
        }
        const res = await fetch('/api/my', { headers: hdrs });
        if (!res.ok) throw new Error('Failed to fetch capsules');
        
        const capsules = await res.json();
        if (capsules && capsules.length > 0) {
            document.getElementById('my-capsules-text').textContent = 
                `У тебя ${capsules.length} капсул(а). Создай новую!`;
        }
    } catch (err) {
        console.warn('Silent fail on load capsules:', err);
    }
}

function switchModel(model) {
    currentModelType = model;
    document.querySelectorAll('.model-option').forEach(m => m.classList.remove('active'));
    document.querySelector(`.model-option[data-model="${model}"]`).classList.add('active');

    const mp = getModelPaths(model);
    const preview = document.getElementById('model-preview');
    if (preview) {
        preview.setAttribute('src', mp.closed);
    }
}

const CDN_URL = "https://github.com/akywaa/time-capsule-tma/releases/download/v1.0.0";

function getModelPaths(modelType) {
    const models = {
        'safe': { closed: `${CDN_URL}/safe.glb`, open: `${CDN_URL}/safe-open.glb` },
        'love': { closed: `${CDN_URL}/love_box.glb`, open: `${CDN_URL}/love_box.glb` },
        'scifi': { closed: `${CDN_URL}/sci-fi_box.glb`, open: `${CDN_URL}/sci-fi_box.glb` }
    };
    return models[modelType] || models['safe'];
}

function setSafeModelSrc(newSrc) {
    const safeModel = document.getElementById('safe-model');
    const overlay = document.getElementById('loading-overlay');
    if (!safeModel) return;

    // Нормализуем URL для сравнения
    const currentSrc = safeModel.getAttribute('src') || safeModel.src;
    
    // Если этот же файл уже отображается — не включаем лоадер повторно
    if (currentSrc === newSrc && safeModel.loaded) {
        overlay.classList.add('hidden');
        return;
    }

    overlay.classList.remove('hidden');

    let isDone = false;
    const hideOverlay = () => {
        if (!isDone) {
            isDone = true;
            overlay.classList.add('hidden');
        }
    };

    // Слушаем и успешную загрузку, и возможную ошибку
    safeModel.addEventListener('load', hideOverlay, { once: true });
    safeModel.addEventListener('error', (e) => {
        console.error('Ошибка загрузки 3D модели:', newSrc, e);
        hideOverlay(); // скрываем лоадер даже при ошибке, чтобы интерфейс не зависал
    }, { once: true });

    // Предохранитель: если за 4 секунды события не пришли, принудительно скрываем спиннер
    setTimeout(hideOverlay, 4000);

    safeModel.setAttribute('src', newSrc);
}

function switchCapsuleType(type) {
    currentCapsuleType = type;
    document.querySelectorAll('.capsule-type-tab').forEach(t => t.classList.remove('active'));
    document.querySelector(`.capsule-type-tab[data-type="${type}"]`).classList.add('active');

    document.getElementById('group-timer').classList.toggle('hidden', type === 'geo');
    document.getElementById('group-stars-section').classList.toggle('hidden', type !== 'group');
    document.getElementById('geo-section-create').classList.toggle('hidden', type !== 'geo');
    document.getElementById('hack-settings').classList.toggle('hidden', type === 'geo');

    const btn = document.getElementById('create-btn');
    if (type === 'group') {
        btn.textContent = 'Создать групповую капсулу';
        btn.style.background = 'linear-gradient(135deg,#f59e0b,#d97706)';
    } else if (type === 'geo') {
        btn.textContent = 'Закопать гео-капсулу';
        btn.style.background = 'linear-gradient(135deg,#10b981,#059669)';
    } else {
        btn.textContent = 'Спрятать в сейф';
        btn.style.background = '';
    }
}

function toggleTimerDropdown(e) {
    e.stopPropagation();
    const dd = document.getElementById('timer-dropdown');
    if (!dd.classList.contains('hidden')) {
        closeAllDropdowns();
        return;
    }
    closeAllDropdowns();
    document.getElementById('timer-select').classList.add('open');
    dd.classList.remove('hidden');
}

function selectTimerOption(el) {
    selectedHours = parseInt(el.dataset.value);
    document.getElementById('timer-label').textContent = el.dataset.label;
    document.querySelectorAll('#timer-dropdown .custom-option').forEach(o => o.classList.remove('selected'));
    el.classList.add('selected');
    closeAllDropdowns();
}

function closeAllDropdowns() {
    document.getElementById('timer-select').classList.remove('open');
    document.getElementById('timer-dropdown').classList.add('hidden');
}

document.addEventListener('click', closeAllDropdowns);

function switchMediaType(type) {
    currentMediaType = type;
    document.querySelectorAll('.media-type-tab').forEach(t => t.classList.remove('active'));
    document.getElementById(`tab-${type}`).classList.add('active');
    document.getElementById('group-text').classList.toggle('hidden', type !== 'text');
    document.getElementById('group-photo').classList.toggle('hidden', type !== 'photo');
    document.getElementById('group-voice').classList.toggle('hidden', type !== 'voice');
}

function previewPhoto(event) {
    const file = event.target.files[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (e) => {
        photoBase64 = e.target.result;
        const img = document.getElementById('photo-preview-img');
        img.src = photoBase64;
        img.classList.remove('hidden');
        document.getElementById('photo-label').textContent = '📎 Нажми чтобы заменить фото';
    };
    reader.readAsDataURL(file);
}

async function toggleRecording() {
    const btn = document.getElementById('voice-btn');
    const preview = document.getElementById('voice-preview-audio');

    if (mediaRecorder && mediaRecorder.state === 'recording') {
        mediaRecorder.stop();
        btn.classList.remove('recording');
        btn.textContent = '🎤';
        clearInterval(voiceTimerInterval);
        return;
    }

    try {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
        const chunks = [];
        mediaRecorder = new MediaRecorder(stream, { mimeType: 'audio/webm' });

        mediaRecorder.ondataavailable = e => chunks.push(e.data);
        mediaRecorder.onstop = () => {
            audioBlob = new Blob(chunks, { type: 'audio/webm' });
            const reader = new FileReader();
            reader.onload = () => {
                voiceBase64 = reader.result;
                preview.src = voiceBase64;
                preview.classList.remove('hidden');
            };
            reader.readAsDataURL(audioBlob);
            stream.getTracks().forEach(t => t.stop());
        };

        mediaRecorder.start();
        btn.classList.add('recording');
        btn.textContent = '⏹️';
        voiceStartTime = Date.now();

        voiceTimerInterval = setInterval(() => {
            const elapsed = Math.floor((Date.now() - voiceStartTime) / 1000);
            const m = Math.floor(elapsed / 60).toString().padStart(2, '0');
            const s = (elapsed % 60).toString().padStart(2, '0');
            document.getElementById('voice-timer').textContent = `${m}:${s}`;
        }, 200);
    } catch (err) {
        console.error('Audio record error:', err);
        tg.showAlert('Нет доступа к микрофону. Разреши запись в настройках.');
    }
}

function fetchLocation() {
    const btn = document.getElementById('geo-fetch-btn');
    btn.classList.add('fetching');
    btn.textContent = '📍 Определяем...';

    if (!navigator.geolocation) {
        btn.classList.remove('fetching');
        btn.textContent = '📍 Определить местоположение';
        return tg.showAlert('Геолокация не поддерживается');
    }

    navigator.geolocation.getCurrentPosition(
        pos => {
            document.getElementById('geo-lat').value = pos.coords.latitude.toFixed(6);
            document.getElementById('geo-lng').value = pos.coords.longitude.toFixed(6);
            document.getElementById('geo-search-input').value = '';
            btn.classList.remove('fetching');
            btn.textContent = '✅ Готово (ваше местоположение)';
            tg.HapticFeedback.notificationOccurred('success');
        },
        err => {
            console.error('Geolocation error:', err);
            btn.classList.remove('fetching');
            btn.textContent = '📍 Определить местоположение';
            tg.showAlert('Не удалось. Разреши доступ в настройках.');
        },
        { enableHighAccuracy: true, timeout: 10000 }
    );
}

function toggleHackPrice() {
    const allow = document.getElementById('allow-hack').checked;
    const row = document.getElementById('row-price');
    row.style.opacity = allow ? '1' : '0.3';
    row.style.pointerEvents = allow ? 'all' : 'none';
}

async function createCapsule() {
    const senderId = tg.initDataUnsafe?.user?.id || 123456789;
    const passcode = document.getElementById('passcode-input').value.replace(/\D/g, '');
    const allowHack = currentCapsuleType === 'geo' 
        ? false 
        : document.getElementById('allow-hack').checked;
    
    const hackPrice = parseInt(document.getElementById('hack-price').value) || 50;
    const goalStars = parseInt(document.getElementById('goal-stars').value) || 500;
    const geoLat = parseFloat(document.getElementById('geo-lat').value) || 0;
    const geoLng = parseFloat(document.getElementById('geo-lng').value) || 0;
    const geoRadius = parseInt(document.getElementById('geo-radius').value) || 50;

    let content = '';
    if (currentMediaType === 'text') {
        content = document.getElementById('text-input').value.trim();
        if (!content) return tg.showAlert("Напиши секрет!");
    } else if (currentMediaType === 'photo') {
        if (!photoBase64) return tg.showAlert("Выбери фото!");
        content = photoBase64;
    } else if (currentMediaType === 'voice') {
        if (!voiceBase64) return tg.showAlert("Запиши голосовое!");
        content = voiceBase64;
    }

    if (currentCapsuleType === 'geo' && (geoLat === 0 || geoLng === 0)) {
        return tg.showAlert("Сначала определи геопозицию!");
    }

    const btn = document.getElementById('create-btn');
    btn.disabled = true;
    const origText = btn.textContent;
    btn.innerHTML = '<span style="opacity:0.7">Создаём...</span>';

    try {
        const payload = {
            sender_id: senderId,
            content,
            hours: currentCapsuleType === 'geo' ? 876000 : selectedHours,
            passcode: passcode || '',
            media_type: currentMediaType,
            allow_hack: allowHack,
            allow_hack_set: true,
            hack_price: hackPrice,
            capsule_type: currentCapsuleType,
            goal_stars: goalStars,
            geo_lat: geoLat,
            geo_lng: geoLng,
            geo_radius: geoRadius,
            model_type: currentModelType
        };

        const res = await fetch('/api/create', {
            method: 'POST',
            headers: apiHeaders(),
            body: JSON.stringify(payload)
        });
        
        if (!res.ok) throw new Error('API Error');
        const data = await res.json();

        const shareUrl = `https://t.me/${BOT_USERNAME}/app?startapp=${data.id}`;
        let msg = 'Я спрятал капсулу времени! Открой, если дождешься.';
        if (currentCapsuleType === 'group') msg = 'Я создал групповую капсулу! Скидываемся?';
        if (currentCapsuleType === 'geo') msg = 'Я спрятал гео-капсулу! Найди место.';

        tg.openTelegramLink(`https://t.me/share/url?url=${encodeURIComponent(shareUrl)}&text=${encodeURIComponent(msg)}`);
        
        cancelCreation();
        document.getElementById('text-input').value = '';
        document.getElementById('passcode-input').value = '';
    } catch (err) {
        console.error('Failed to create capsule:', err);
        tg.showAlert("Ошибка сервера.");
    } finally {
        btn.disabled = false;
        btn.innerHTML = origText;
    }
}

async function loadCapsule(id) {
    try {
        const oldBlur = document.getElementById('blur-preview');
        if (oldBlur) oldBlur.remove();
        if (geoWatchId) {
            navigator.geolocation.clearWatch(geoWatchId);
            geoWatchId = null;
        }

        const viewerId = tg.initDataUnsafe?.user?.id;
        const url = `/api/get?id=${id}${viewerId ? '&viewer_id=' + viewerId : ''}`;
        
        const res = await fetch(url);
        if (!res.ok) throw new Error('Capsule not found');

        const data = await res.json();
        capsuleData = data;
        clearInterval(timerInterval);

        const isOpen = data.is_hacked || (data.capsule_type !== 'geo' && new Date() >= new Date(data.unlock_at));
        const modelPaths = getModelPaths(data.model_type || 'safe');

        setSafeModelSrc(isOpen ? modelPaths.open : modelPaths.closed);

        updateReactions(data.reactions || {});

        const badge = document.getElementById('type-badge');
        badge.classList.add('hidden');
        if (data.capsule_type === 'group') {
            badge.textContent = '👥 Группа';
            badge.className = 'capsule-type-badge group';
        } else if (data.capsule_type === 'geo') {
            badge.textContent = '📍 Гео';
            badge.className = 'capsule-type-badge geo';
        }

        const ps = document.getElementById('passcode-section');
        if (data.has_passcode && !isOpen) {
            ps.classList.remove('hidden');
            const attemptsEl = document.getElementById('passcode-attempts-text');
            attemptsEl.textContent = `Осталось попыток: ${data.passcode_attempts || 3}`;
            if (data.passcode_attempts <= 0) {
                document.getElementById('passcode-submit-btn').disabled = true;
                attemptsEl.textContent = 'Попытки закончились.';
                attemptsEl.classList.add('warn');
            }
        } else {
            ps.classList.add('hidden');
        }

        const hackBtn = document.getElementById('hack-btn');
        const groupCard = document.getElementById('group-drop-card');
        const geoCard = document.getElementById('geo-view-card');
        const timerCard = document.getElementById('timer-card');

        hackBtn.classList.add('hidden');
        groupCard.classList.add('hidden');
        geoCard.classList.add('hidden');

        if (isOpen) {
            timerCard.classList.add('hidden');
            document.getElementById('reveal-section').classList.remove('hidden');
            fireConfetti();
        } else if (data.capsule_type === 'group') {
            timerCard.classList.add('hidden');
            groupCard.classList.remove('hidden');
            renderGroupProgress(data);
        } else if (data.capsule_type === 'geo') {
            timerCard.classList.add('hidden');
            geoCard.classList.remove('hidden');
            startGeoWatch(data);
        } else {
            timerCard.classList.remove('hidden');
            document.getElementById('timer-label-text').textContent = 'Сейф откроется через';
            document.getElementById('timer-status-text').textContent = 'ЗАКРЫТ';

            if (data.allow_hack !== false) {
                hackBtn.classList.remove('hidden');
                hackBtn.querySelector('.star-icon').nextSibling.textContent = ` Взломать досрочно (${data.hack_price || 50} Stars)`;
            }
        }

        if (data.preview && data.media_type === 'photo') {
            const container = document.getElementById('model-container');
            let blurOverlay = document.getElementById('blur-preview');
            if (!blurOverlay) {
                blurOverlay = document.createElement('div');
                blurOverlay.id = 'blur-preview';
                blurOverlay.style.cssText = 'position:absolute;inset:0;z-index:5;display:flex;align-items:center;justify-content:center;pointer-events:none';
                container.appendChild(blurOverlay);
            }
            blurOverlay.innerHTML = buildBlurredPhoto(data.content);
            blurOverlay.classList.remove('hidden');
        }

        if (data.capsule_type === 'personal' && !isOpen) {
            const updateTimer = () => {
                const now = new Date();
                const unlockAt = new Date(data.unlock_at);
                const diff = unlockAt - now;

                if (diff <= 0 || data.is_hacked) {
                    clearInterval(timerInterval);
                    const mp = getModelPaths(data.model_type || 'safe');
                    setSafeModelSrc(mp.open);
                    
                    document.getElementById('status-container').classList.add('hidden');
                    document.getElementById('passcode-section').classList.add('hidden');
                    document.getElementById('reveal-section').classList.remove('hidden');
                    tg.HapticFeedback?.notificationOccurred?.('success');
                } else {
                    const h = Math.floor(diff / 36e5).toString().padStart(2, '0');
                    const m = Math.floor((diff % 36e5) / 6e4).toString().padStart(2, '0');
                    const s = Math.floor((diff % 6e4) / 1e3).toString().padStart(2, '0');
                    document.getElementById('timer').innerText = `${h}:${m}:${s}`;
                }
            };
            updateTimer();
            timerInterval = setInterval(updateTimer, 1000);
        }
    } catch (err) {
        console.error('Load capsule failed:', err);
        tg.showAlert("Ошибка загрузки.");
    }
}

function renderGroupProgress(data) {
    const contributions = data.stars_contributions || {};
    const goal = data.goal_stars || 500;
    
    let total = 0;
    for (const amt of Object.values(contributions)) total += amt;

    const pct = Math.min(100, Math.round((total / goal) * 100));
    document.getElementById('group-progress-fill').style.width = `${pct}%`;
    document.getElementById('group-progress-text').textContent = `${total} / ${goal} ⭐`;
    document.getElementById('group-progress-pct').textContent = `${pct}%`;

    const list = document.getElementById('contributor-list');
    list.innerHTML = '';
    
    for (const [uid, amt] of Object.entries(contributions)) {
        const pill = document.createElement('span');
        pill.className = 'contributor-pill';
        pill.textContent = `🆔${uid.slice(-4)}: ${amt}⭐`;
        list.appendChild(pill);
    }

    if (data.is_hacked) {
        const btn = document.getElementById('contribute-btn');
        btn.disabled = true;
        btn.textContent = '✅ Взломан!';
    }
}

async function contributeStars() {
    if (!currentCapsuleId) return;
    const uid = tg.initDataUnsafe?.user?.id;
    
    if (!uid) return tg.showAlert('Необходим Telegram ID');
    tg.HapticFeedback.impactOccurred('medium');

    try {
        const res = await fetch('/api/contribute', {
            method: 'POST',
            headers: apiHeaders(),
            body: JSON.stringify({ id: currentCapsuleId, user_id: uid, amount: 50 })
        });
        
        if (!res.ok) throw new Error('API Error');
        const d = await res.json();
        
        capsuleData.stars_contributions = d.stars_contributions;
        capsuleData.is_hacked = d.is_hacked;
        renderGroupProgress(capsuleData);

        if (d.is_hacked) {
            fireConfetti();
            tg.showAlert('🎉 Капсула взломана!');
            loadCapsule(currentCapsuleId);
        }
    } catch (err) {
        console.error('Contribute failed:', err);
        tg.showAlert('Ошибка платежа.');
    }
}

function startGeoWatch(data) {
    if (!navigator.geolocation) {
        document.getElementById('geo-status-text').textContent = 'Геолокация не поддерживается';
        return;
    }

    document.getElementById('geo-status-text').textContent = 'Поиск позиции...';

    if (geoMap) {
        if (geoMarker) geoMap.removeLayer(geoMarker);
        if (userMarker) geoMap.removeLayer(userMarker);
        userMarker = null;
        geoMap.setView([data.geo_lat, data.geo_lng], 15);
        geoMap.eachLayer(l => {
            if (l instanceof L.Circle) geoMap.removeLayer(l);
        });
    } else {
        geoMap = L.map('geo-map', { zoomControl: true, attributionControl: false })
            .setView([data.geo_lat, data.geo_lng], 15);
        L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', { maxZoom: 19 }).addTo(geoMap);
    }

    const targetIcon = L.divIcon({
        html: '<div style="width:24px;height:24px;border-radius:50%;background:rgba(16,185,129,0.3);border:3px solid #10b981;box-shadow:0 0 16px rgba(16,185,129,0.5);display:flex;align-items:center;justify-content:center;font-size:14px">📍</div>',
        className: '',
        iconSize: [24, 24],
        iconAnchor: [12, 12]
    });
    
    geoMarker = L.marker([data.geo_lat, data.geo_lng], { icon: targetIcon }).addTo(geoMap);
    geoMarker.bindPopup(`<b>🎯 Цель</b><br>Радиус: ${data.geo_radius || 50}м`).openPopup();

    L.circle([data.geo_lat, data.geo_lng], {
        radius: data.geo_radius || 50,
        color: '#10b981',
        fillColor: '#10b981',
        fillOpacity: 0.1,
        weight: 1,
        dashArray: '6,8'
    }).addTo(geoMap);

    setTimeout(() => {
        if (geoMap) geoMap.invalidateSize();
    }, 100);

    const checkDistance = (pos) => {
        lastGeoPos = pos;
        const lat = pos.coords.latitude;
        const lng = pos.coords.longitude;

        if (geoMap) {
            if (!userMarker) {
                const userIcon = L.divIcon({
                    html: '<div style="width:16px;height:16px;border-radius:50%;background:#22d3ee;border:3px solid #fff;box-shadow:0 0 12px rgba(34,211,238,0.8)"></div>',
                    className: '',
                    iconSize: [16, 16],
                    iconAnchor: [8, 8]
                });
                userMarker = L.marker([lat, lng], { icon: userIcon }).addTo(geoMap);
            } else {
                userMarker.setLatLng([lat, lng]);
                geoMap.setView([lat, lng], geoMap.getZoom());
            }
        }

        const R = 6371000;
        const dLat = (data.geo_lat - lat) * Math.PI / 180;
        const dLng = (data.geo_lng - lng) * Math.PI / 180;
        const a = Math.sin(dLat / 2) ** 2 +
            Math.cos(lat * Math.PI / 180) * Math.cos(data.geo_lat * Math.PI / 180) *
            Math.sin(dLng / 2) ** 2;
        const dist = Math.round(R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a)));

        document.getElementById('geo-distance').textContent = dist;

        const dot = document.getElementById('geo-status-dot');
        const st = document.getElementById('geo-status-text');
        const ub = document.getElementById('geo-unlock-btn');

        if (dist <= (data.geo_radius || 50)) {
            dot.className = 'geo-status-dot unlocked';
            st.textContent = 'Ты на месте!';
            ub.classList.remove('hidden');
        } else if (dist < 200) {
            dot.className = 'geo-status-dot near';
            st.textContent = 'Совсем рядом!';
            ub.classList.add('hidden');
        } else {
            dot.className = 'geo-status-dot';
            st.textContent = `До цели ${dist}м`;
            ub.classList.add('hidden');
        }
    };

    navigator.geolocation.getCurrentPosition(
        checkDistance,
        () => { document.getElementById('geo-status-text').textContent = 'Ошибка позиции'; },
        { enableHighAccuracy: true }
    );
    
    geoWatchId = navigator.geolocation.watchPosition(checkDistance, null, {
        enableHighAccuracy: true,
        maximumAge: 5000
    });
}

async function unlockGeo() {
    if (!capsuleData || !lastGeoPos) {
        return tg.showAlert('Сначала дождись определения позиции!');
    }

    try {
        const res = await fetch('/api/geo-check', {
            method: 'POST',
            headers: apiHeaders(),
            body: JSON.stringify({
                id: currentCapsuleId,
                lat: lastGeoPos.coords.latitude,
                lng: lastGeoPos.coords.longitude
            })
        });
        
        if (!res.ok) throw new Error('Network error');
        const d = await res.json();

        if (d.unlocked || capsuleData.is_hacked) {
            capsuleData.is_hacked = true;
            document.getElementById('geo-unlock-btn').disabled = true;
            document.getElementById('geo-unlock-btn').textContent = '✅ Открыто!';
            fireConfetti();
            loadCapsule(currentCapsuleId);
        } else {
            tg.showAlert(`Не на месте! Расстояние: ${Math.round(d.distance)}м`);
        }
    } catch (err) {
        console.error('Geo unlock failed:', err);
        tg.showAlert('Ошибка при проверке геопозиции.');
    }
}

async function revealSecret() {
    const model = document.getElementById('safe-model');
    const section = document.getElementById('reveal-section');
    const flash = document.getElementById('safe-flash');
    const popup = document.getElementById('secret-popup');
    const reactionsBar = document.getElementById('reactions-bar');
    const data = capsuleData;

    if (!data) return;

    section.querySelector('.reveal-title').innerHTML = 'Открываем<span class="dots-pulse"><span class="dots"></span></span>';
    section.querySelector('.reveal-sub').classList.add('hidden');
    section.querySelector('.reveal-btn').classList.add('hidden');
    reactionsBar.classList.add('hidden');

    tg.HapticFeedback.impactOccurred('heavy');

    model.removeAttribute('auto-rotate');
    model.cameraOrbit = '0deg 75deg 3m';
    await sleep(600);

    model.classList.add('zooming');
    model.cameraOrbit = '0deg 60deg 3m';
    // model.cameraOrbit = '0deg 90deg 2m'; // старый ракурс, а то хрень какая то
    await sleep(200);

    model.setAttribute('field-of-view', '30deg');
    model.cameraOrbit = '0deg 80deg 1.5m';
    await sleep(400);

    model.setAttribute('field-of-view', '12deg');
    model.cameraOrbit = '10deg 85deg 0.6m';
    await sleep(500);

    flash.style.opacity = '1';
    await sleep(150);
    flash.style.opacity = '0';
    await sleep(100);

    model.classList.remove('zooming');
    model.setAttribute('field-of-view', '45deg');
    model.cameraOrbit = '45deg 75deg 4m';
    model.setAttribute('auto-rotate', '');
    section.classList.add('hidden');

    popup.classList.remove('hidden');
    document.getElementById('secret-content').innerHTML = buildContent(data);
    fireConfetti();
}

function closeSecret() {
    document.getElementById('secret-popup').classList.add('hidden');
    document.getElementById('reactions-bar').classList.remove('hidden');
    document.getElementById('reveal-section').classList.remove('hidden');

    const section = document.getElementById('reveal-section');
    section.querySelector('.reveal-title').textContent = 'Время вышло! 🔓';
    section.querySelector('.reveal-sub').classList.remove('hidden');
    section.querySelector('.reveal-btn').classList.remove('hidden');
}

function shareSecret() {
    const shareUrl = `https://t.me/${BOT_USERNAME}/app?startapp=${currentCapsuleId}`;
    tg.openTelegramLink(`https://t.me/share/url?url=${encodeURIComponent(shareUrl)}&text=${encodeURIComponent("Загляни в эту капсулу времени!")}`);
}

function createOwn() {
    document.getElementById('secret-popup').classList.add('hidden');
    document.getElementById('view-view').classList.add('hidden');
    document.getElementById('create-view').classList.remove('hidden');
    currentCapsuleId = null;
    tg.HapticFeedback.impactOccurred('light');
}

const sleep = (ms) => new Promise(r => setTimeout(r, ms));

function buildContent(data) {
    if (data.media_type === 'photo') {
        return `<img src="${data.content}" class="secret-photo" alt="Секретное фото">`;
    } else if (data.media_type === 'voice') {
        return `<audio controls class="secret-audio" src="${data.content}"></audio>`;
    }
    return `<div class="secret-content">${escapeHtml(data.content)}</div>`;
}

function buildBlurredPhoto(src) {
    return `<div class="photo-blur-wrapper"><img src="${src}" class="secret-photo blurred" alt="Размытое фото"><div class="blur-badge">👁 Размыто до открытия</div></div>`;
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function fireConfetti() {
    confetti({
        particleCount: 150,
        spread: 80,
        origin: { y: 0.5, x: 0.5 },
        colors: ['#06b6d4', '#22d3ee', '#f59e0b', '#8b5cf6', '#22c55e', '#ef4444']
    });
    setTimeout(() => confetti({
        particleCount: 80,
        spread: 100,
        origin: { y: 0.4, x: 0.3 },
        colors: ['#06b6d4', '#f59e0b', '#22c55e']
    }), 200);
    setTimeout(() => confetti({
        particleCount: 80,
        spread: 100,
        origin: { y: 0.4, x: 0.7 },
        colors: ['#22d3ee', '#8b5cf6', '#ef4444']
    }), 400);
}

function updateReactions(reactions) {
    ['👀', '😈', '🥺', '🔥', '💎'].forEach(emoji => {
        const rc = document.getElementById(`rc-${emoji}`);
        const prc = document.getElementById(`prc-${emoji}`);
        const val = reactions[emoji] || 0;
        
        if (rc) rc.textContent = val;
        if (prc) prc.textContent = val;
    });
}

const apiHeaders = () => {
    const headers = { 'Content-Type': 'application/json' };
    if (tg.initData) headers['X-Telegram-Init-Data'] = tg.initData;
    return headers;
};

async function sendReaction(emoji) {
    if (!currentCapsuleId) return;
    const userId = tg.initDataUnsafe?.user?.id;
    if (!userId) return;

    tg.HapticFeedback.impactOccurred('light');

    try {
        const res = await fetch('/api/reaction', {
            method: 'POST',
            headers: apiHeaders(),
            body: JSON.stringify({ id: currentCapsuleId, emoji, user_id: userId })
        });
        const data = await res.json();
        updateReactions(data.reactions || {});
    } catch (err) {
        console.warn('Reaction update failed:', err);
    }
}

function digitInput(e, idx) {
    const val = e.target.value.replace(/\D/g, '');
    e.target.value = val.slice(-1);
    if (val && idx < 3) {
        document.getElementById(`pd-${idx + 1}`)?.focus();
    }
}

function digitKeydown(e, idx) {
    if (e.key === 'Backspace' && !e.target.value && idx > 0) {
        document.getElementById(`pd-${idx - 1}`)?.focus();
    }
}

async function tryPasscode() {
    const digits = [];
    for (let i = 0; i < 4; i++) {
        digits.push(document.getElementById(`pd-${i}`).value);
    }
    const code = digits.join('');

    if (code.length < 4) {
        return tg.showAlert('Введи все 4 цифры!');
    }

    try {
        const res = await fetch('/api/passcode', {
            method: 'POST',
            headers: apiHeaders(),
            body: JSON.stringify({ id: currentCapsuleId, passcode: code })
        });
        const data = await res.json();

        if (data.success) {
            tg.HapticFeedback.notificationOccurred('success');
            document.getElementById('passcode-section').classList.add('hidden');
            loadCapsule(currentCapsuleId);
        } else {
            tg.HapticFeedback.notificationOccurred('error');
            const attemptsText = document.getElementById('passcode-attempts-text');
            attemptsText.textContent = `Осталось попыток: ${data.attempts}`;

            if (data.attempts <= 0) {
                document.getElementById('passcode-submit-btn').disabled = true;
                attemptsText.textContent = 'Попытки закончились. Только платный взлом!';
                attemptsText.classList.add('warn');
            }

            for (let i = 0; i < 4; i++) document.getElementById(`pd-${i}`).value = '';
            document.getElementById('pd-0').focus();

            tg.showAlert(`Неверный код. ${data.attempts > 0 ? `Осталось ${data.attempts} попыток.` : 'Попытки закончились!'}`);
        }
    } catch (err) {
        console.error('Passcode check failed:', err);
        tg.showAlert('Ошибка сети.');
    }
}

async function hackCapsule() {
    if (capsuleData && capsuleData.allow_hack === false) {
        return tg.showAlert('Создатель запретил взлом за звёзды.');
    }

    tg.HapticFeedback.impactOccurred('medium');

    try {
        const res = await fetch(`/api/invoice?id=${currentCapsuleId}`);
        const data = await res.json();

        if (data.url) {
            tg.openInvoice(data.url, (status) => {
                if (status === 'paid' || status === 'pending') {
                    tg.showAlert("Успех! Взломан");
                    loadCapsule(currentCapsuleId);
                }
            });
        }
    } catch (err) {
        console.error('Hack invoice failed:', err);
        tg.showAlert("Ошибка генерации счета.");
    }
}

async function searchGeoPlace() {
    const q = document.getElementById('geo-search-input').value.trim();
    const resultsEl = document.getElementById('geo-search-results');

    if (geoSearchTimeout) clearTimeout(geoSearchTimeout);

    if (q.length < 2) {
        resultsEl.style.display = 'none';
        return;
    }

    geoSearchTimeout = setTimeout(async () => {
        try {
            const res = await fetch(`https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(q)}&limit=5&accept-language=ru`, { 
                headers: { 'User-Agent': 'CapsuleBot/1.0' } 
            });
            const places = await res.json();

            if (!places.length) {
                resultsEl.style.display = 'none';
                return;
            }

            resultsEl.innerHTML = '';
            places.forEach(p => {
                const div = document.createElement('div');
                div.className = 'geo-result-item';
                div.textContent = '📍 ' + p.display_name;
                
                // норм навешивание ивента
                div.addEventListener('click', () => {
                    // console.log('Выбрали место:', p.display_name); // TODO: убрать из прода
                    document.getElementById('geo-lat').value = parseFloat(p.lat).toFixed(6);
                    document.getElementById('geo-lng').value = parseFloat(p.lon).toFixed(6);
                    document.getElementById('geo-search-input').value = p.display_name;
                    resultsEl.style.display = 'none';
                    
                    let btnText = p.display_name.substring(0, 30);
                    if (p.display_name.length > 30) btnText += '...';
                    document.getElementById('geo-fetch-btn').textContent = `✅ Выбрано: ${btnText}`;
                    
                    tg.HapticFeedback.notificationOccurred('success');
                });
                
                resultsEl.appendChild(div);
            });
            resultsEl.style.display = 'block';
        } catch (err) {
            console.warn('Geo search failed:', err);
            resultsEl.style.display = 'none';
        }
    }, 400);
}



document.addEventListener('click', (e) => {
    if (!e.target.closest('#geo-search-input') && !e.target.closest('#geo-search-results')) {
        document.getElementById('geo-search-results').style.display = 'none';
    }
});
