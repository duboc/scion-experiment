/*
 * Copyright 2025 Google LLC
 * 
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * 
 *     https://www.apache.org/licenses/LICENSE-2.0
 * 
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import axios from 'axios';
import config from '../config';

const API_URL = config.apiUrl;

/**
 * Retrieve admin token strictly from session storage.
 * Never reads or touches localStorage.
 */
function getAdminToken(explicitToken) {
  if (explicitToken && typeof explicitToken === 'string') {
    return explicitToken.trim();
  }
  try {
    return (sessionStorage.getItem('psearch_admin_token') || '').trim();
  } catch {
    return '';
  }
}

function getAuthHeaders(token) {
  const adminToken = getAdminToken(token);
  const headers = {};
  if (adminToken) {
    headers['X-Admin-Token'] = adminToken;
  }
  return headers;
}

function normalizeTimestamp(val) {
  if (!val || typeof val !== 'string' || !val.trim()) {
    return undefined;
  }
  const trimmed = val.trim();
  const parsed = new Date(trimmed);
  if (!isNaN(parsed.getTime())) {
    return parsed.toISOString();
  }
  return trimmed;
}

/**
 * Map an API ad campaign object to the UI campaign representation.
 */
export function fromApiCampaign(apiCampaign) {
  if (!apiCampaign) return null;
  const cpcMicros = typeof apiCampaign.cpc_micros === 'number'
    ? apiCampaign.cpc_micros
    : (parseInt(apiCampaign.cpc_micros, 10) || 0);
  const budgetMicros = typeof apiCampaign.budget_micros === 'number'
    ? apiCampaign.budget_micros
    : (parseInt(apiCampaign.budget_micros, 10) || 0);
  const spentMicros = typeof apiCampaign.spent_micros === 'number'
    ? apiCampaign.spent_micros
    : (parseInt(apiCampaign.spent_micros, 10) || 0);

  return {
    id: apiCampaign.campaign_id || apiCampaign.id || '',
    campaign_id: apiCampaign.campaign_id || apiCampaign.id || '',
    tenant_id: apiCampaign.tenant_id || 'default',
    advertiser: apiCampaign.advertiser || '',
    product_id: apiCampaign.product_id || '',
    cpc_micros: cpcMicros,
    budget_micros: budgetMicros,
    spent_micros: spentMicros,
    status: apiCampaign.status || 'active',
    start_at: apiCampaign.start_at || null,
    end_at: apiCampaign.end_at || null,
    updated_at: apiCampaign.updated_at || null,
  };
}

/**
 * Map a UI ad campaign object to the API payload representation.
 */
export function toApiCampaign(uiCampaign) {
  const tenant_id = (uiCampaign.tenant_id || 'default').trim() || 'default';
  const campaign_id = (uiCampaign.campaign_id || uiCampaign.id || '').trim();
  const advertiser = (uiCampaign.advertiser || '').trim().slice(0, 128);
  const product_id = (uiCampaign.product_id || '').trim().slice(0, 64);
  const cpc_micros = Math.round(Number(uiCampaign.cpc_micros) || 0);
  const budget_micros = Math.round(Number(uiCampaign.budget_micros) || 0);
  const spent_micros = Math.round(Number(uiCampaign.spent_micros) || 0);
  const status = uiCampaign.status || 'active';

  const payload = {
    campaign_id,
    tenant_id,
    advertiser,
    product_id,
    cpc_micros,
    budget_micros,
    spent_micros,
    status,
  };

  const startAt = normalizeTimestamp(uiCampaign.start_at);
  if (startAt) {
    payload.start_at = startAt;
  }
  const endAt = normalizeTimestamp(uiCampaign.end_at);
  if (endAt) {
    payload.end_at = endAt;
  }

  return payload;
}

/**
 * Send fire-and-forget click beacon for a sponsored search result.
 * Never awaits or blocks navigation.
 */
export function recordAdClick(campaignId, clickId) {
  if (!campaignId || !clickId) return;
  const payload = {
    campaign_id: String(campaignId),
    click_id: String(clickId),
  };
  try {
    if (typeof window !== 'undefined' && typeof window.fetch === 'function') {
      window.fetch(`${API_URL}/api/ads/click`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        keepalive: true,
      }).catch(() => {});
      return;
    }
    axios.post(`${API_URL}/api/ads/click`, payload).catch(() => {});
  } catch {
    // fire-and-forget: never block navigation
  }
}

export const adService = {
  /**
   * Get all ad campaigns for a tenant via GET /api/ads/campaigns
   * @param {string} tenantId - Optional tenant ID (defaults to 'default')
   * @returns {Promise<Array>} Array of mapped campaign objects
   */
  async getCampaigns(tenantId = 'default') {
    const res = await axios.get(`${API_URL}/api/ads/campaigns`, {
      params: { tenant_id: tenantId || 'default' },
    });
    const list = (res.data && (res.data.campaigns || (Array.isArray(res.data) ? res.data : []))) || [];
    return list.map(fromApiCampaign).filter(Boolean);
  },

  /**
   * Alias for getCampaigns
   */
  async listCampaigns(tenantId = 'default') {
    return this.getCampaigns(tenantId);
  },

  /**
   * Create a new ad campaign via POST /api/ads/campaigns
   * @param {Object} campaign - Campaign object
   * @param {string} token - Optional X-Admin-Token
   * @returns {Promise<Object>} Created campaign
   */
  async createCampaign(campaign, token) {
    const apiPayload = toApiCampaign(campaign);
    const tenantId = apiPayload.tenant_id || 'default';
    const res = await axios.post(`${API_URL}/api/ads/campaigns`, apiPayload, {
      params: { tenant_id: tenantId },
      headers: getAuthHeaders(token),
    });
    return fromApiCampaign((res.data && res.data.campaign) || res.data);
  },

  /**
   * Update an existing ad campaign via PUT /api/ads/campaigns/:id
   * @param {string} campaignId - ID of campaign
   * @param {Object} campaign - Updated campaign object
   * @param {string} token - Optional X-Admin-Token
   * @returns {Promise<Object>} Updated campaign
   */
  async updateCampaign(campaignId, campaign, token) {
    const apiPayload = toApiCampaign(campaign);
    const tenantId = apiPayload.tenant_id || 'default';
    const res = await axios.put(`${API_URL}/api/ads/campaigns/${encodeURIComponent(campaignId)}`, apiPayload, {
      params: { tenant_id: tenantId },
      headers: getAuthHeaders(token),
    });
    return fromApiCampaign((res.data && res.data.campaign) || res.data);
  },

  /**
   * Delete an ad campaign via DELETE /api/ads/campaigns/:id
   * @param {string} campaignId - ID of campaign to delete
   * @param {string} tenantId - Optional tenant ID
   * @param {string} token - Optional X-Admin-Token
   * @returns {Promise<void>}
   */
  async deleteCampaign(campaignId, tenantId = 'default', token) {
    let tid = tenantId;
    let tkn = token;
    if (typeof tenantId === 'string' && tenantId.length > 25 && !token) {
      tkn = tenantId;
      tid = 'default';
    }
    await axios.delete(`${API_URL}/api/ads/campaigns/${encodeURIComponent(campaignId)}`, {
      params: { tenant_id: tid || 'default' },
      headers: getAuthHeaders(tkn),
    });
  },

  /**
   * Fire-and-forget click beacon via POST /api/ads/click
   */
  recordClick(campaignId, clickId) {
    recordAdClick(campaignId, clickId);
  },

  /**
   * Check health status of serving service
   * @returns {Promise<Object>}
   */
  async checkHealth() {
    try {
      const res = await axios.get(`${API_URL}/api/health`);
      return res.data || { status: 'healthy' };
    } catch {
      try {
        const res2 = await axios.get(`${API_URL}/health`);
        return res2.data || { status: 'healthy' };
      } catch {
        return { status: 'available' };
      }
    }
  },
};

export default adService;
