// Flight Log App - Frontend JavaScript

(function() {
    'use strict';

    // State
    let userEmail = localStorage.getItem('flightlog_email') || '';
    let extractedFlight = null;
    let currentImageFile = null;
    let selectedModel = localStorage.getItem('flightlog_model') || '';
    let availableModels = [];

    // DOM Elements
    const emailScreen = document.getElementById('emailScreen');
    const appScreen = document.getElementById('appScreen');
    const emailForm = document.getElementById('emailForm');
    const emailInput = document.getElementById('emailInput');
    const userEmailDisplay = document.getElementById('userEmail');
    const addFlightBtn = document.getElementById('addFlightBtn');
    const emptyAddBtn = document.getElementById('emptyAddBtn');
    const loadSampleBtn = document.getElementById('loadSampleBtn');
    const emptyState = document.getElementById('emptyState');
    const flightsList = document.getElementById('flightsList');
    const addFlightModal = document.getElementById('addFlightModal');
    const closeModal = document.getElementById('closeModal');
    const uploadZone = document.getElementById('uploadZone');
    const fileInput = document.getElementById('fileInput');
    const extractionStatus = document.getElementById('extractionStatus');
    const continueSection = document.getElementById('continueSection');
    const continueBtn = document.getElementById('continueBtn');
    const extractedData = document.getElementById('extractedData');
    const previewImage = document.getElementById('previewImage');
    const cancelSave = document.getElementById('cancelSave');
    const saveFlight = document.getElementById('saveFlight');
    const signOutBtn = document.getElementById('signOutBtn');
    const modelSelect = document.getElementById('modelSelect');

    // All Flights Section DOM Elements
    const allFlightsSection = document.getElementById('allFlightsSection');
    const allFlightsToggle = document.getElementById('allFlightsToggle');
    const allFlightsBody = document.getElementById('allFlightsBody');
    const allFlightsTable = document.getElementById('allFlightsTable');

    // All Flights Section State
    let allFlightsLoaded = false;
    let allFlightsData = [];
    let sortColumn = 'departureDate';
    let sortDirection = 'desc';

    // Sample Gallery DOM Elements
    const samplesSection = document.getElementById('samplesSection');
    const samplesGallery = document.getElementById('samplesGallery');
    const samplePreview = document.getElementById('samplePreview');
    const samplePreviewImage = document.getElementById('samplePreviewImage');
    const samplePreviewCancel = document.getElementById('samplePreviewCancel');
    const samplePreviewUse = document.getElementById('samplePreviewUse');

    // Sample Gallery State
    let selectedSampleUrl = null;

    // Initialize
    function init() {
        loadModels(); // Fetch available models
        if (userEmail) {
            showApp();
            loadFlights();
        } else {
            showEmailScreen();
        }
        bindEvents();
    }

    // Event Bindings
    function bindEvents() {
        // Email form
        emailForm.addEventListener('submit', handleEmailSubmit);

        // Add flight buttons
        addFlightBtn.addEventListener('click', openModal);
        emptyAddBtn.addEventListener('click', openModal);
        loadSampleBtn.addEventListener('click', handleLoadSampleData);

        // Sign out
        signOutBtn.addEventListener('click', handleSignOut);

        // Modal
        closeModal.addEventListener('click', closeModalHandler);
        addFlightModal.addEventListener('click', (e) => {
            if (e.target === addFlightModal) closeModalHandler();
        });

        // Upload zone
        uploadZone.addEventListener('click', () => fileInput.click());
        uploadZone.addEventListener('dragover', handleDragOver);
        uploadZone.addEventListener('dragleave', handleDragLeave);
        uploadZone.addEventListener('drop', handleDrop);
        fileInput.addEventListener('change', handleFileSelect);

        // Save/Cancel
        cancelSave.addEventListener('click', closeModalHandler);
        saveFlight.addEventListener('click', handleSaveFlight);

        // Keyboard
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && addFlightModal.classList.contains('active')) {
                closeModalHandler();
            }
            if (e.key === 'Escape' && samplePreview.classList.contains('active')) {
                hideSamplePreview();
            }
        });

        // Sample preview
        samplePreviewCancel.addEventListener('click', hideSamplePreview);
        samplePreviewUse.addEventListener('click', handleUseSample);
        samplePreview.addEventListener('click', (e) => {
            if (e.target === samplePreview) hideSamplePreview();
        });

        // Model selection
        modelSelect.addEventListener('change', handleModelChange);
    }

    // ============================================================================
    // Model Selection
    // ============================================================================

    async function loadModels() {
        try {
            const response = await fetch('/api/models');
            if (!response.ok) {
                console.error('Failed to fetch models');
                return;
            }
            const data = await response.json();
            availableModels = data.models || [];

            // Use stored model if valid, otherwise use server default
            if (!selectedModel || !availableModels.find(m => m.id === selectedModel)) {
                selectedModel = data.defaultModel || '';
                localStorage.setItem('flightlog_model', selectedModel);
            }

            renderModelDropdown();
            console.log(`[MODELS] Loaded ${availableModels.length} models. Selected: ${selectedModel}`);
        } catch (error) {
            console.error('Error loading models:', error);
        }
    }

    function renderModelDropdown() {
        modelSelect.innerHTML = '';

        if (availableModels.length === 0) {
            const opt = document.createElement('option');
            opt.value = '';
            opt.textContent = 'No models available';
            modelSelect.appendChild(opt);
            return;
        }

        // Group: Free models (multiplier = 0)
        const freeModels = availableModels.filter(m => m.multiplier === 0);
        const premiumModels = availableModels.filter(m => m.multiplier > 0);

        if (freeModels.length > 0) {
            const freeGroup = document.createElement('optgroup');
            freeGroup.label = '✓ Included';
            freeModels.forEach(model => {
                freeGroup.appendChild(createModelOption(model));
            });
            modelSelect.appendChild(freeGroup);
        }

        if (premiumModels.length > 0) {
            const premiumGroup = document.createElement('optgroup');
            premiumGroup.label = '⚡ Premium';
            premiumModels.forEach(model => {
                premiumGroup.appendChild(createModelOption(model));
            });
            modelSelect.appendChild(premiumGroup);
        }
    }

    function createModelOption(model) {
        const option = document.createElement('option');
        option.value = model.id;
        option.textContent = `${model.name} ${model.costLabel}`;
        option.selected = model.id === selectedModel;
        return option;
    }

    function handleModelChange(e) {
        selectedModel = e.target.value;
        localStorage.setItem('flightlog_model', selectedModel);
        console.log(`[MODELS] Changed to: ${selectedModel}`);
    }

    // Screen Management
    function showEmailScreen() {
        emailScreen.style.display = 'flex';
        appScreen.classList.remove('active');
        emailInput.focus();
    }

    function showApp() {
        emailScreen.style.display = 'none';
        appScreen.classList.add('active');
        userEmailDisplay.textContent = userEmail;
    }

    // Email Submit
    function handleEmailSubmit(e) {
        e.preventDefault();
        const email = emailInput.value.trim();
        if (email && validateEmail(email)) {
            userEmail = email;
            localStorage.setItem('flightlog_email', email);
            showApp();
            loadFlights();
        }
    }

    function validateEmail(email) {
        return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
    }

    // Sign Out
    function handleSignOut() {
        userEmail = '';
        localStorage.removeItem('flightlog_email');
        extractedFlight = null;
        currentImageFile = null;
        // Clear query results from previous user
        clearQueryResults();
        // Clear All Flights section to prevent cross-user data leakage
        clearAllFlightsState();
        showEmailScreen();
    }

    // Clear All Flights state (used on sign out to prevent cross-user data leakage)
    function clearAllFlightsState() {
        allFlightsLoaded = false;
        allFlightsData = [];
        if (allFlightsTable) {
            allFlightsTable.innerHTML = '';
        }
        if (allFlightsBody) {
            allFlightsBody.classList.add('hidden');
        }
        if (allFlightsToggle) {
            const arrow = allFlightsToggle.querySelector('.all-flights-arrow');
            if (arrow) arrow.textContent = '▶';
        }
        if (allFlightsSection) {
            allFlightsSection.classList.add('hidden');
        }
        // Clear flight count
        updateFlightCount(0);
    }

    // Clear query results (used on sign out to prevent cross-user data leakage)
    function clearQueryResults() {
        const queryResult = document.getElementById('queryResult');
        const queryResultContent = document.getElementById('queryResultContent');
        const querySQLCode = document.getElementById('querySQLCode');
        const querySection = document.getElementById('querySection');
        const flightsList = document.getElementById('flightsList');
        const emptyState = document.getElementById('emptyState');
        
        if (queryResult) queryResult.classList.add('hidden');
        if (queryResultContent) queryResultContent.textContent = '';
        if (querySQLCode) querySQLCode.textContent = '';
        if (querySection) querySection.classList.add('hidden');
        if (flightsList) {
            flightsList.innerHTML = '';
            flightsList.classList.add('hidden');
        }
        if (emptyState) emptyState.classList.add('hidden');
    }

    // Modal Management
    function openModal() {
        addFlightModal.classList.add('active');
        resetModal();
        loadSamples();
    }

    function closeModalHandler() {
        addFlightModal.classList.remove('active');
        resetModal();
    }

    function resetModal() {
        uploadZone.style.display = 'block';
        if (samplesSection) samplesSection.style.display = 'block';
        extractionStatus.classList.remove('active');
        extractedData.classList.remove('active');
        extractedFlight = null;
        currentImageFile = null;
        fileInput.value = '';
        
        // Reset progress steps
        const steps = extractionStatus.querySelectorAll('.progress-step');
        steps.forEach(step => {
            const stepNum = step.dataset.step;
            step.classList.remove('active', 'completed');
            step.style.color = '';
            step.style.opacity = '';
            const indicator = step.querySelector('.step-indicator');
            if (indicator) indicator.textContent = stepNum;
            // Clear step details
            const detailEl = step.querySelector('.step-detail');
            if (detailEl) {
                detailEl.textContent = '';
                detailEl.classList.remove('visible');
            }
        });
        
        // Hide continue section
        continueSection.classList.remove('visible');
        continueSection.style.display = 'none';
        
        // Remove any error messages
        const errorDiv = extractionStatus.querySelector('.extraction-error');
        if (errorDiv) errorDiv.remove();
    }

    // Drag & Drop
    function handleDragOver(e) {
        e.preventDefault();
        uploadZone.classList.add('dragover');
    }

    function handleDragLeave(e) {
        e.preventDefault();
        uploadZone.classList.remove('dragover');
    }

    function handleDrop(e) {
        e.preventDefault();
        uploadZone.classList.remove('dragover');
        const files = e.dataTransfer.files;
        if (files.length > 0) {
            processFile(files[0]);
        }
    }

    function handleFileSelect(e) {
        const files = e.target.files;
        if (files.length > 0) {
            processFile(files[0]);
        }
    }

    // File Processing
    function processFile(file) {
        if (!file.type.startsWith('image/')) {
            alert('Please upload an image file');
            return;
        }

        currentImageFile = file;

        // Show preview
        const reader = new FileReader();
        reader.onload = (e) => {
            previewImage.src = e.target.result;
        };
        reader.readAsDataURL(file);

        // Start extraction
        uploadZone.style.display = 'none';
        if (samplesSection) samplesSection.style.display = 'none';
        extractionStatus.classList.add('active');
        
        // Initialize progress: Step 1 is uploading (active)
        updateProgressStep(1, 'active');

        extractFlightData(file);
    }

    // API: Extract Flight Data
    async function extractFlightData(file) {
        const formData = new FormData();
        formData.append('image', file);
        formData.append('model', selectedModel);

        try {
            const response = await fetch('/api/extract', {
                method: 'POST',
                headers: {
                    'X-User-Email': userEmail
                },
                body: formData
            });

            if (!response.ok) {
                throw new Error('Failed to extract flight data');
            }

            // Handle SSE stream
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';
            let currentEventType = 'message';

            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                buffer += decoder.decode(value, { stream: true });
                const lines = buffer.split('\n');
                buffer = lines.pop() || '';

                for (const line of lines) {
                    if (line.startsWith('event: ')) {
                        currentEventType = line.slice(7).trim();
                        continue;
                    }
                    if (line.startsWith('data: ')) {
                        const data = line.slice(6);
                        handleSSEEvent(currentEventType, data);
                        currentEventType = 'message'; // Reset for next event
                    }
                }
            }
        } catch (error) {
            console.error('Extraction error:', error);
            showExtractionError(error.message);
        }
    }

    function handleSSEEvent(eventType, data) {
        if (eventType === 'step') {
            try {
                const stepData = JSON.parse(data);
                updateProgressStep(stepData.step, stepData.status, stepData.detail);
            } catch (e) {
                console.error('Failed to parse step data:', e);
            }
            return;
        }

        if (eventType === 'error') {
            showExtractionError(data);
            return;
        }

        if (eventType === 'extracted') {
            try {
                const flight = JSON.parse(data);
                if (flight.flightNumber || flight.fromAirport) {
                    extractedFlight = flight;
                    // Mark step 4 as completed
                    updateProgressStep(4, 'completed');
                    // Show continue button for demo pause
                    showContinueButton(() => showExtractedData(flight));
                }
            } catch (e) {
                console.error('Failed to parse extracted data:', e);
            }
        }
    }

    function updateProgressStep(stepNumber, status, detail) {
        const steps = extractionStatus.querySelectorAll('.progress-step');
        
        steps.forEach(step => {
            const stepNum = parseInt(step.dataset.step);
            const indicator = step.querySelector('.step-indicator');
            const detailEl = step.querySelector('.step-detail');
            
            if (stepNum < stepNumber) {
                // Completed steps
                step.classList.remove('active');
                step.classList.add('completed');
                indicator.textContent = '✓';
            } else if (stepNum === stepNumber) {
                // Current step
                if (status === 'active') {
                    step.classList.add('active');
                    step.classList.remove('completed');
                    // Show detail if provided
                    if (detail && detailEl) {
                        detailEl.textContent = detail;
                        detailEl.classList.add('visible');
                    }
                } else if (status === 'completed') {
                    step.classList.remove('active');
                    step.classList.add('completed');
                    indicator.textContent = '✓';
                }
            } else {
                // Future steps
                step.classList.remove('active', 'completed');
                indicator.textContent = stepNum;
            }
        });
    }

    function showContinueButton(onContinue) {
        // Show the continue section with animation
        continueSection.style.display = 'block';
        // Trigger reflow for animation
        continueSection.offsetHeight;
        continueSection.classList.add('visible');
        
        // Set up one-time click handler
        const handleClick = () => {
            continueBtn.removeEventListener('click', handleClick);
            continueSection.classList.remove('visible');
            setTimeout(() => {
                continueSection.style.display = 'none';
                onContinue();
            }, 200);
        };
        continueBtn.addEventListener('click', handleClick);
    }

    function showExtractionError(message) {
        // Mark all incomplete steps as failed
        const steps = extractionStatus.querySelectorAll('.progress-step:not(.completed)');
        steps.forEach(step => {
            step.style.color = 'var(--red-alert)';
            step.style.opacity = '1';
        });
        
        // Add error message
        const errorDiv = document.createElement('div');
        errorDiv.className = 'extraction-error';
        errorDiv.style.cssText = 'color: var(--red-alert); font-family: var(--font-display); font-size: 0.875rem; margin-top: var(--space-lg); text-align: center;';
        errorDiv.textContent = 'Error: ' + message;
        extractionStatus.appendChild(errorDiv);
        
        setTimeout(resetModal, 3000);
    }

    // Legacy handler for backward compatibility
    function handleSSEData(data) {
        handleSSEEvent('message', data);
    }

    function showExtractedData(flight) {
        extractionStatus.classList.remove('active');
        extractedData.classList.add('active');

        document.getElementById('extractedFlight').textContent = flight.flightNumber || '-';
        document.getElementById('extractedRoute').textContent = 
            (flight.fromAirport && flight.toAirport) 
                ? `${flight.fromAirport} → ${flight.toAirport}` 
                : '-';
        document.getElementById('extractedDate').textContent = formatDate(flight.departureDate) || '-';
        document.getElementById('extractedTime').textContent = flight.departureTime || '-';
        document.getElementById('extractedSeat').textContent = flight.seat || '-';
        document.getElementById('extractedGate').textContent = flight.gate || '-';
        document.getElementById('extractedPassenger').textContent = flight.passenger || '-';
    }

    // API: Save Flight
    async function handleSaveFlight() {
        if (!extractedFlight) return;

        saveFlight.disabled = true;
        saveFlight.textContent = 'Saving...';

        try {
            const response = await fetch('/api/flights', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    ...extractedFlight,
                    email: userEmail
                })
            });

            if (!response.ok) {
                throw new Error('Failed to save flight');
            }

            closeModalHandler();
            loadFlights();
            // Refresh All Flights if section is visible
            await refreshAllFlightsIfVisible();
        } catch (error) {
            console.error('Save error:', error);
            alert('Failed to save flight: ' + error.message);
        } finally {
            saveFlight.disabled = false;
            saveFlight.textContent = 'Save Flight';
        }
    }

    // API: Load Flights
    async function loadFlights() {
        try {
            const response = await fetch(`/api/flights?email=${encodeURIComponent(userEmail)}`);
            if (!response.ok) {
                throw new Error('Failed to load flights');
            }

            const flights = await response.json();
            renderFlights(flights || []);
        } catch (error) {
            console.error('Load error:', error);
        }
    }

    // API: Delete Flight
    async function deleteFlight(id) {
        if (!confirm('Are you sure you want to delete this flight?')) return;

        try {
            const response = await fetch(`/api/flights/${id}?email=${encodeURIComponent(userEmail)}`, {
                method: 'DELETE'
            });

            if (!response.ok) {
                throw new Error('Failed to delete flight');
            }

            loadFlights();
            // Refresh All Flights if section is visible
            await refreshAllFlightsIfVisible();
        } catch (error) {
            console.error('Delete error:', error);
            alert('Failed to delete flight: ' + error.message);
        }
    }

    // Refresh All Flights data if the section is expanded
    async function refreshAllFlightsIfVisible() {
        if (allFlightsBody && !allFlightsBody.classList.contains('hidden')) {
            await loadAllFlights();
        } else {
            allFlightsLoaded = false; // Mark stale for next expand
        }
    }

    // API: Load Sample Data
    async function handleLoadSampleData() {
        loadSampleBtn.disabled = true;
        loadSampleBtn.textContent = 'Loading...';

        try {
            const response = await fetch(`/api/sample?email=${encodeURIComponent(userEmail)}`, {
                method: 'POST'
            });

            if (!response.ok) {
                throw new Error('Failed to load sample data');
            }

            loadFlights();
            // Refresh All Flights if section is visible
            await refreshAllFlightsIfVisible();
        } catch (error) {
            console.error('Sample data error:', error);
            alert('Failed to load sample data: ' + error.message);
        } finally {
            loadSampleBtn.disabled = false;
            loadSampleBtn.textContent = 'Load Sample Data';
        }
    }

    // Render Flights (limited to 3 recent)
    function renderFlights(flights) {
        if (flights.length === 0) {
            emptyState.classList.remove('hidden');
            flightsList.classList.add('hidden');
            hideQuerySection();
            hideAllFlightsSection();
            return;
        }

        emptyState.classList.add('hidden');
        flightsList.classList.remove('hidden');
        showQuerySection();
        showAllFlightsSection();

        // Limit to 3 recent flights for compact view
        const displayFlights = flights.slice(0, 3);

        // Add section header + flight cards
        const headerHTML = `
            <div class="flights-header">
                <h2>✈️ Recent Flights</h2>
                <p class="flights-hint">Use AI chat below to explore your flight history</p>
            </div>
        `;

        flightsList.innerHTML = headerHTML + displayFlights.map(flight => {
            const date = parseDate(flight.departureDate);
            return `
                <div class="flight-card" data-id="${flight.id}">
                    <div class="flight-card-content">
                        <div class="flight-date">
                            <div class="flight-date-day">${date.day}</div>
                            <div class="flight-date-month">${date.month}</div>
                        </div>
                        <div class="flight-details">
                            <div class="flight-route">
                                <span>${flight.fromAirport || '???'}</span>
                                <span class="flight-route-arrow">→</span>
                                <span>${flight.toAirport || '???'}</span>
                            </div>
                            <div class="flight-meta">
                                <span>
                                    <span class="flight-meta-label">Flight</span>
                                    ${flight.flightNumber || '-'}
                                </span>
                                <span>
                                    <span class="flight-meta-label">Seat</span>
                                    ${flight.seat || '-'}
                                </span>
                                <span>
                                    <span class="flight-meta-label">Gate</span>
                                    ${flight.gate || '-'}
                                </span>
                            </div>
                            <div class="flight-secondary">
                                ${flight.airline ? `<span class="flight-airline">${flight.airline}</span>` : ''}
                                ${flight.departureTime ? `<span class="flight-time">✈ ${flight.departureTime}</span>` : ''}
                                ${flight.passenger ? `<span class="flight-passenger">${flight.passenger}</span>` : ''}
                            </div>
                        </div>
                        <div class="flight-actions">
                            <button class="btn btn-danger" onclick="window.flightLog.deleteFlight('${flight.id}')" aria-label="Delete flight">
                                🗑️
                            </button>
                        </div>
                    </div>
                </div>
            `;
        }).join('');
    }

    // Utility Functions
    function formatDate(dateStr) {
        if (!dateStr) return '';
        try {
            const date = new Date(dateStr + 'T00:00:00');
            return date.toLocaleDateString('en-US', { 
                month: 'short', 
                day: 'numeric', 
                year: 'numeric' 
            });
        } catch {
            return dateStr;
        }
    }

    function parseDate(dateStr) {
        if (!dateStr) return { day: '--', month: '---' };
        try {
            const date = new Date(dateStr + 'T00:00:00');
            return {
                day: date.getDate(),
                month: date.toLocaleDateString('en-US', { month: 'short' })
            };
        } catch {
            return { day: '--', month: '---' };
        }
    }

    // ===== AI QUERY FUNCTIONALITY =====
    const querySection = document.getElementById('querySection');
    const queryInput = document.getElementById('queryInput');
    const querySubmit = document.getElementById('querySubmit');
    const queryResult = document.getElementById('queryResult');
    const queryResultContent = document.getElementById('queryResultContent');
    const queryResultClose = document.getElementById('queryResultClose');
    const queryGeneratedSQL = document.getElementById('queryGeneratedSQL');
    const querySQLCode = document.getElementById('querySQLCode');
    const queryLoading = document.getElementById('queryLoading');
    const queryExamples = document.querySelectorAll('.query-example');
    const queryHeaderToggle = document.getElementById('queryHeaderToggle');

    // Toggle collapsible AI section
    if (queryHeaderToggle) {
        queryHeaderToggle.addEventListener('click', () => {
            querySection.classList.toggle('collapsed');
            // Save preference to localStorage
            localStorage.setItem('flightlog_ai_collapsed', querySection.classList.contains('collapsed'));
        });
        
        // Restore collapsed state from localStorage
        const isCollapsed = localStorage.getItem('flightlog_ai_collapsed') === 'true';
        if (isCollapsed) {
            querySection.classList.add('collapsed');
        }
    }

    // Example query clicks
    queryExamples.forEach(example => {
        example.addEventListener('click', () => {
            queryInput.value = example.dataset.query;
            submitQuery();
        });
    });

    // Query submit
    if (querySubmit) {
        querySubmit.addEventListener('click', submitQuery);
        queryInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') submitQuery();
        });
    }

    // Close result
    if (queryResultClose) {
        queryResultClose.addEventListener('click', () => {
            queryResult.classList.add('hidden');
        });
    }

    async function submitQuery() {
        const question = queryInput.value.trim();
        if (!question || !userEmail) return;

        // Disable input while processing
        queryInput.disabled = true;
        querySubmit.disabled = true;
        queryResult.classList.add('hidden');
        queryLoading.classList.remove('hidden');

        try {
            const response = await fetch('/api/chat', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-User-Email': userEmail
                },
                body: JSON.stringify({ message: question, model: selectedModel })
            });

            if (!response.ok) {
                throw new Error('Query request failed');
            }

            // Read SSE stream
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let aiResponse = '';
            let generatedQuery = '';

            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                const text = decoder.decode(value);
                const lines = text.split('\n');

                for (const line of lines) {
                    if (line.startsWith('data: ')) {
                        const data = line.substring(6);
                        
                        // Check for query event (the SQL)
                        if (data.startsWith('SELECT') || data.includes('FROM c')) {
                            generatedQuery = data;
                        }
                        
                        // Check for final response
                        try {
                            const parsed = JSON.parse(data);
                            if (parsed.message) {
                                aiResponse = parsed.message;
                                generatedQuery = parsed.query || generatedQuery;
                            }
                        } catch {
                            // Not JSON, might be a delta
                        }
                    }
                }
            }

            // Show result
            queryLoading.classList.add('hidden');
            queryResultContent.textContent = aiResponse || 'No response received';
            
            if (generatedQuery) {
                querySQLCode.textContent = generatedQuery;
                queryGeneratedSQL.classList.remove('hidden');
            } else {
                queryGeneratedSQL.classList.add('hidden');
            }
            
            queryResult.classList.remove('hidden');

        } catch (error) {
            console.error('Query error:', error);
            queryLoading.classList.add('hidden');
            queryResultContent.textContent = 'Sorry, I encountered an error processing your request. Please try again.';
            queryGeneratedSQL.classList.add('hidden');
            queryResult.classList.remove('hidden');
        } finally {
            queryInput.disabled = false;
            querySubmit.disabled = false;
            queryInput.value = '';
            queryInput.focus();
        }
    }

    // Show query section when flights are loaded
    function showQuerySection() {
        if (querySection) {
            querySection.classList.remove('hidden');
        }
    }

    function hideQuerySection() {
        if (querySection) {
            querySection.classList.add('hidden');
        }
    }

    // ===== ALL FLIGHTS SECTION =====

    if (allFlightsToggle) {
        allFlightsToggle.addEventListener('click', toggleAllFlights);
    }

    // Setup sortable headers
    document.querySelectorAll('.all-flights-table th[data-sort]').forEach(th => {
        th.addEventListener('click', () => handleSort(th.dataset.sort));
    });

    function showAllFlightsSection() {
        if (allFlightsSection) {
            allFlightsSection.classList.remove('hidden');
        }
    }

    function hideAllFlightsSection() {
        if (allFlightsSection) {
            allFlightsSection.classList.add('hidden');
        }
    }

    async function toggleAllFlights() {
        if (!allFlightsBody) return;

        const isExpanded = !allFlightsBody.classList.contains('hidden');
        
        if (isExpanded) {
            // Collapse
            allFlightsBody.classList.add('hidden');
            allFlightsToggle.querySelector('.all-flights-arrow').textContent = '▶';
        } else {
            // Expand and load if needed
            if (!allFlightsLoaded) {
                await loadAllFlights();
            }
            allFlightsBody.classList.remove('hidden');
            allFlightsToggle.querySelector('.all-flights-arrow').textContent = '▼';
        }
    }

    async function loadAllFlights() {
        try {
            allFlightsTable.innerHTML = '<tr><td colspan="7" class="loading-cell">Loading...</td></tr>';
            
            const response = await fetch(`/api/flights/all?email=${encodeURIComponent(userEmail)}`);
            if (!response.ok) throw new Error('Failed to load flights');
            
            const flights = await response.json();
            allFlightsData = flights || [];
            renderAllFlightsTable(allFlightsData);
            allFlightsLoaded = true;
        } catch (error) {
            console.error('Failed to load all flights:', error);
            allFlightsTable.innerHTML = '<tr><td colspan="7" class="error-cell">Failed to load flights</td></tr>';
        }
    }

    function handleSort(column) {
        // Toggle direction if same column, otherwise default to ascending
        if (sortColumn === column) {
            sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
        } else {
            sortColumn = column;
            sortDirection = 'asc';
        }

        // Sort the data
        const sorted = [...allFlightsData].sort((a, b) => {
            let valA = getSortValue(a, column);
            let valB = getSortValue(b, column);
            
            if (valA < valB) return sortDirection === 'asc' ? -1 : 1;
            if (valA > valB) return sortDirection === 'asc' ? 1 : -1;
            return 0;
        });

        renderAllFlightsTable(sorted);
        updateSortIndicators();
    }

    function getSortValue(flight, column) {
        switch (column) {
            case 'departureDate': return flight.departureDate || '';
            case 'flightNumber': return flight.flightNumber || '';
            case 'route': return (flight.fromAirport || '') + (flight.toAirport || '');
            case 'airline': return flight.airline || '';
            case 'seat': return flight.seat || '';
            case 'passenger': return flight.passenger || '';
            default: return '';
        }
    }

    function updateSortIndicators() {
        document.querySelectorAll('.all-flights-table th[data-sort]').forEach(th => {
            const indicator = th.querySelector('.sort-indicator');
            if (indicator) {
                if (th.dataset.sort === sortColumn) {
                    indicator.textContent = sortDirection === 'asc' ? '▲' : '▼';
                } else {
                    indicator.textContent = '⇅';
                }
            }
        });
    }

    function renderAllFlightsTable(flights) {
        // Update flight count display
        updateFlightCount(flights.length);

        if (flights.length === 0) {
            allFlightsTable.innerHTML = '<tr><td colspan="7" class="empty-cell">No flights found</td></tr>';
            return;
        }

        allFlightsTable.innerHTML = flights.map(flight => `
            <tr>
                <td>${flight.departureDate || '-'}</td>
                <td>${flight.flightNumber || '-'}</td>
                <td>${flight.fromAirport || '-'} → ${flight.toAirport || '-'}</td>
                <td>${flight.airline || '-'}</td>
                <td>${flight.seat || '-'}</td>
                <td>${flight.passenger || '-'}</td>
                <td class="actions-cell">
                    <button class="btn-delete-small" onclick="window.flightLog.deleteFlight('${flight.id}')" title="Delete flight">🗑️</button>
                </td>
            </tr>
        `).join('');
    }

    function updateFlightCount(count) {
        const countEl = document.getElementById('allFlightsCount');
        if (countEl) {
            countEl.textContent = count > 0 ? `(${count})` : '';
        }
    }

    // ===== SAMPLE GALLERY =====

    async function loadSamples() {
        try {
            const response = await fetch('/api/samples');
            if (!response.ok) return;
            const samples = await response.json();
            renderSampleThumbnails(samples);
        } catch (error) {
            console.error('Failed to load samples:', error);
        }
    }

    function renderSampleThumbnails(samples) {
        if (!samplesGallery) return;
        
        if (!samples || samples.length === 0) {
            if (samplesSection) samplesSection.style.display = 'none';
            return;
        }

        samplesGallery.innerHTML = samples.map(src => `
            <img src="${src}" class="sample-thumb" alt="Sample boarding pass" data-src="${src}">
        `).join('');

        // Add click handlers
        samplesGallery.querySelectorAll('.sample-thumb').forEach(thumb => {
            thumb.addEventListener('click', () => {
                showSamplePreview(thumb.dataset.src);
            });
        });
    }

    function showSamplePreview(url) {
        selectedSampleUrl = url;
        samplePreviewImage.src = url;
        samplePreview.classList.add('active');
    }

    function hideSamplePreview() {
        samplePreview.classList.remove('active');
        selectedSampleUrl = null;
        samplePreviewImage.src = '';
    }

    async function handleUseSample() {
        if (!selectedSampleUrl) return;

        try {
            // Fetch the image and convert to File
            const response = await fetch(selectedSampleUrl);
            const blob = await response.blob();
            const filename = selectedSampleUrl.split('/').pop();
            const file = new File([blob], filename, { type: blob.type });

            // Hide preview and samples section
            hideSamplePreview();
            if (samplesSection) samplesSection.style.display = 'none';

            // Process the file (existing flow)
            processFile(file);
        } catch (error) {
            console.error('Failed to use sample:', error);
            alert('Failed to load sample image');
        }
    }

    // Expose delete function for inline handlers
    window.flightLog = {
        deleteFlight: deleteFlight
    };

    // Start app
    init();
})();
