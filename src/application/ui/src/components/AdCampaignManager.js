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

import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  Grid,
  IconButton,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  TextField,
  Typography,
  Alert,
  CircularProgress,
  Chip,
} from '@mui/material';
import { LoadingButton } from '@mui/lab';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { adService } from '../services/adService';

const CAMPAIGN_STATUSES = {
  ACTIVE: 'active',
  PAUSED: 'paused',
  EXHAUSTED: 'exhausted',
};

function formatCurrencyFromMicros(micros) {
  const n = Number(micros) || 0;
  return `$${(n / 1000000).toFixed(2)}`;
}

function getStatusChipColor(status) {
  switch (String(status || '').toLowerCase()) {
    case CAMPAIGN_STATUSES.ACTIVE:
      return 'success';
    case CAMPAIGN_STATUSES.PAUSED:
      return 'warning';
    case CAMPAIGN_STATUSES.EXHAUSTED:
      return 'error';
    default:
      return 'default';
  }
}

export default function AdCampaignManager({ tenantId: propTenantId = 'default' }) {
  const [campaigns, setCampaigns] = useState([]);
  const [loading, setLoading] = useState(false);
  const [openDialog, setOpenDialog] = useState(false);
  const [editingCampaign, setEditingCampaign] = useState(null);
  const [error, setError] = useState(null);
  const [apiHealth, setApiHealth] = useState(null);
  const activeTenantId = propTenantId || 'default';

  // Admin token stored ONLY in sessionStorage (never localStorage, never logged)
  const [adminToken, setAdminToken] = useState(() => {
    try {
      return sessionStorage.getItem('psearch_admin_token') || '';
    } catch {
      return '';
    }
  });

  const handleAdminTokenChange = (e) => {
    const val = e.target.value;
    setAdminToken(val);
    try {
      sessionStorage.setItem('psearch_admin_token', val);
    } catch {}
  };

  const [currentCampaign, setCurrentCampaign] = useState({
    campaign_id: '',
    tenant_id: activeTenantId,
    advertiser: '',
    product_id: '',
    cpc_micros: 250000,
    budget_micros: 5000000,
    spent_micros: 0,
    status: CAMPAIGN_STATUSES.ACTIVE,
    start_at: '',
    end_at: '',
  });

  const checkApiHealth = useCallback(async () => {
    try {
      const health = await adService.checkHealth();
      setApiHealth(health);
      setError(null);
    } catch {
      setApiHealth(null);
      setError('API Connection Error: Ad campaign service unavailable');
    }
  }, []);

  const fetchCampaigns = useCallback(async () => {
    setLoading(true);
    try {
      const list = await adService.getCampaigns(activeTenantId);
      setCampaigns(list || []);
      setError(null);
    } catch (err) {
      if (err.response) {
        if (err.response.status === 403) {
          setError('403 Forbidden: Invalid or missing admin token.');
        } else if (err.response.status === 400) {
          const msg = err.response.data?.error || err.response.data?.message || 'Invalid request';
          setError(`400 Bad Request: ${msg}`);
        } else {
          setError(`Error (${err.response.status}): ${err.response.data?.error || err.message}`);
        }
      } else {
        setError('Failed to fetch ad campaigns. Please try again.');
      }
      setCampaigns([]);
    } finally {
      setLoading(false);
    }
  }, [activeTenantId]);

  useEffect(() => {
    checkApiHealth();
    fetchCampaigns();
  }, [checkApiHealth, fetchCampaigns]);

  const handleOpenDialog = (campaign = null) => {
    if (campaign) {
      setEditingCampaign(campaign);
      setCurrentCampaign({
        campaign_id: campaign.campaign_id || campaign.id || '',
        tenant_id: campaign.tenant_id || activeTenantId,
        advertiser: campaign.advertiser || '',
        product_id: campaign.product_id || '',
        cpc_micros: campaign.cpc_micros !== undefined ? campaign.cpc_micros : 250000,
        budget_micros: campaign.budget_micros !== undefined ? campaign.budget_micros : 5000000,
        spent_micros: campaign.spent_micros !== undefined ? campaign.spent_micros : 0,
        status: campaign.status || CAMPAIGN_STATUSES.ACTIVE,
        start_at: campaign.start_at || '',
        end_at: campaign.end_at || '',
      });
    } else {
      setEditingCampaign(null);
      setCurrentCampaign({
        campaign_id: '',
        tenant_id: activeTenantId,
        advertiser: '',
        product_id: '',
        cpc_micros: 250000,
        budget_micros: 5000000,
        spent_micros: 0,
        status: CAMPAIGN_STATUSES.ACTIVE,
        start_at: '',
        end_at: '',
      });
    }
    setOpenDialog(true);
  };

  const handleCloseDialog = () => {
    setOpenDialog(false);
    setEditingCampaign(null);
    setError(null);
  };

  const validateCampaign = (campaign) => {
    if (!campaign.product_id || !String(campaign.product_id).trim()) {
      throw new Error('Product ID cannot be empty');
    }
    const cpc = Math.round(Number(campaign.cpc_micros));
    if (isNaN(cpc) || cpc <= 0) {
      throw new Error('CPC (cpc_micros) must be greater than 0');
    }
    const budget = Math.round(Number(campaign.budget_micros));
    if (isNaN(budget) || budget < cpc) {
      throw new Error('Budget (budget_micros) must be at least equal to cpc_micros');
    }
  };

  const handleSaveCampaign = async () => {
    setLoading(true);
    setError(null);
    try {
      validateCampaign(currentCampaign);

      if (editingCampaign) {
        const id = editingCampaign.campaign_id || editingCampaign.id;
        await adService.updateCampaign(id, currentCampaign, adminToken);
      } else {
        await adService.createCampaign(currentCampaign, adminToken);
      }

      await fetchCampaigns();
      handleCloseDialog();
    } catch (err) {
      if (err.response) {
        if (err.response.status === 403) {
          setError('403 Forbidden: Invalid or missing admin token.');
        } else if (err.response.status === 400) {
          const msg = err.response.data?.error || err.response.data?.message || 'Invalid campaign parameters';
          setError(`400 Bad Request: ${msg}`);
        } else {
          setError(`Error (${err.response.status}): ${err.response.data?.error || err.message}`);
        }
      } else {
        setError(err.message || 'Failed to save campaign. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteCampaign = async (campaignId) => {
    if (window.confirm('Are you sure you want to delete this campaign?')) {
      setLoading(true);
      setError(null);
      try {
        await adService.deleteCampaign(campaignId, activeTenantId, adminToken);
        await fetchCampaigns();
      } catch (err) {
        if (err.response) {
          if (err.response.status === 403) {
            setError('403 Forbidden: Invalid or missing admin token.');
          } else if (err.response.status === 400) {
            const msg = err.response.data?.error || err.response.data?.message || 'Invalid request';
            setError(`400 Bad Request: ${msg}`);
          } else {
            setError(`Error (${err.response.status}): ${err.response.data?.error || err.message}`);
          }
        } else {
          setError(err.message || 'Failed to delete campaign. Please try again.');
        }
      } finally {
        setLoading(false);
      }
    }
  };

  if (loading && campaigns.length === 0) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3, flexWrap: 'wrap', gap: 2 }}>
        <Box>
          <Typography variant="h5" sx={{ mb: 1 }}>Sponsored Campaigns (Retail Media)</Typography>
          <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
            {apiHealth && (
              <Chip
                label={`Status: ${apiHealth.status}`}
                color={apiHealth.status === 'healthy' ? 'success' : 'default'}
                size="small"
              />
            )}
            <Typography variant="caption" color="text.secondary">
              Native ad injection with ACID real-time CPC budget control (Cloud Spanner)
            </Typography>
          </Box>
        </Box>
        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
          <TextField
            type="password"
            size="small"
            label="Admin token"
            id="ad-admin-token-input"
            inputProps={{ 'aria-label': 'Admin token' }}
            value={adminToken}
            onChange={handleAdminTokenChange}
            placeholder="Enter X-Admin-Token"
            helperText="Stored only in session"
            sx={{ minWidth: 200 }}
          />
          <Button
            variant="contained"
            onClick={() => handleOpenDialog()}
            disabled={loading}
          >
            Add New Campaign
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      <Grid container spacing={3}>
        {campaigns.map((campaign) => {
          const cid = campaign.campaign_id || campaign.id;
          const spent = Number(campaign.spent_micros) || 0;
          const budget = Math.max(1, Number(campaign.budget_micros) || 1);
          const pct = Math.min(100, Math.max(0, (spent / budget) * 100));
          const statusStr = String(campaign.status || CAMPAIGN_STATUSES.ACTIVE).toLowerCase();
          const barColor = statusStr === CAMPAIGN_STATUSES.EXHAUSTED || pct >= 100
            ? 'error'
            : pct >= 80
              ? 'warning'
              : 'primary';

          return (
            <Grid item xs={12} md={6} key={cid}>
              <Card variant="outlined" sx={{ borderRadius: 2 }} data-testid="ad-campaign-card">
                <CardContent>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1.5 }}>
                    <Box>
                      <Typography variant="h6" sx={{ fontWeight: 600 }}>
                        {campaign.advertiser || cid}
                      </Typography>
                      <Box sx={{ display: 'flex', gap: 1, mt: 0.5, alignItems: 'center' }}>
                        <Chip
                          label={statusStr.toUpperCase()}
                          color={getStatusChipColor(statusStr)}
                          size="small"
                          data-testid="campaign-status-chip"
                        />
                        <Chip
                          label={`Product: ${campaign.product_id}`}
                          variant="outlined"
                          size="small"
                        />
                      </Box>
                    </Box>
                    <Box>
                      <IconButton onClick={() => handleOpenDialog(campaign)} size="small" aria-label="edit campaign">
                        <EditIcon fontSize="small" />
                      </IconButton>
                      <IconButton onClick={() => handleDeleteCampaign(cid)} size="small" aria-label="delete campaign">
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </Box>
                  </Box>

                  <Typography variant="body2" color="text.secondary">
                    <strong>Campaign ID:</strong> {cid}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    <strong>Advertiser:</strong> {campaign.advertiser || 'N/A'}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    <strong>Product ID:</strong> {campaign.product_id}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    <strong>CPC Bid:</strong> {formatCurrencyFromMicros(campaign.cpc_micros)} ({Number(campaign.cpc_micros || 0).toLocaleString()} micros)
                  </Typography>
                  {(campaign.start_at || campaign.end_at) && (
                    <Typography variant="body2" color="text.secondary">
                      <strong>Window:</strong> {campaign.start_at || 'open'} → {campaign.end_at || 'open'}
                    </Typography>
                  )}

                  {/* Budget bar (spent / budget) */}
                  <Box sx={{ mt: 2 }} data-testid="campaign-budget-bar">
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.5 }}>
                      <Typography variant="caption" sx={{ fontWeight: 600 }}>
                        Budget: {formatCurrencyFromMicros(spent)} / {formatCurrencyFromMicros(campaign.budget_micros)}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {spent.toLocaleString()} / {Number(campaign.budget_micros || 0).toLocaleString()} micros ({pct.toFixed(0)}%)
                      </Typography>
                    </Box>
                    <LinearProgress
                      variant="determinate"
                      value={pct}
                      color={barColor}
                      sx={{ height: 8, borderRadius: 4 }}
                    />
                  </Box>
                </CardContent>
              </Card>
            </Grid>
          );
        })}
      </Grid>

      <Dialog open={openDialog} onClose={handleCloseDialog} maxWidth="sm" fullWidth>
        <DialogTitle>{editingCampaign ? 'Edit Ad Campaign' : 'Create New Ad Campaign'}</DialogTitle>
        <DialogContent>
          <Box sx={{ pt: 1 }}>
            <TextField
              fullWidth
              label="Campaign ID"
              value={currentCampaign.campaign_id}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, campaign_id: e.target.value })}
              disabled={Boolean(editingCampaign)}
              helperText="Leave empty on create to auto-generate, or specify a custom ID"
              sx={{ mb: 2 }}
            />

            <TextField
              fullWidth
              label="Advertiser"
              value={currentCampaign.advertiser}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, advertiser: e.target.value })}
              sx={{ mb: 2 }}
            />

            <TextField
              fullWidth
              label="Product ID"
              value={currentCampaign.product_id}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, product_id: e.target.value })}
              sx={{ mb: 2 }}
              required
            />

            <TextField
              fullWidth
              type="number"
              label="CPC Bid (cpc_micros)"
              value={currentCampaign.cpc_micros}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, cpc_micros: parseInt(e.target.value, 10) || 0 })}
              inputProps={{ min: 1, step: 10000 }}
              helperText="Bid per click in micros (1,000,000 micros = $1.00)"
              sx={{ mb: 2 }}
              required
            />

            <TextField
              fullWidth
              type="number"
              label="Total Budget (budget_micros)"
              value={currentCampaign.budget_micros}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, budget_micros: parseInt(e.target.value, 10) || 0 })}
              inputProps={{ min: 1, step: 100000 }}
              helperText="Total campaign budget in micros (must be >= cpc_micros)"
              sx={{ mb: 2 }}
              required
            />

            <FormControl fullWidth sx={{ mb: 2 }}>
              <InputLabel id="campaign-status-label">Status</InputLabel>
              <Select
                labelId="campaign-status-label"
                value={currentCampaign.status}
                label="Status"
                onChange={(e) => setCurrentCampaign({ ...currentCampaign, status: e.target.value })}
              >
                <MenuItem value={CAMPAIGN_STATUSES.ACTIVE}>Active</MenuItem>
                <MenuItem value={CAMPAIGN_STATUSES.PAUSED}>Paused</MenuItem>
                <MenuItem value={CAMPAIGN_STATUSES.EXHAUSTED}>Exhausted</MenuItem>
              </Select>
            </FormControl>

            <TextField
              fullWidth
              label="Start At (ISO-8601 optional)"
              value={currentCampaign.start_at}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, start_at: e.target.value })}
              placeholder="2026-01-01T00:00:00Z"
              sx={{ mb: 2 }}
            />

            <TextField
              fullWidth
              label="End At (ISO-8601 optional)"
              value={currentCampaign.end_at}
              onChange={(e) => setCurrentCampaign({ ...currentCampaign, end_at: e.target.value })}
              placeholder="2026-12-31T23:59:59Z"
              sx={{ mb: 2 }}
            />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDialog}>Cancel</Button>
          <LoadingButton
            loading={loading}
            onClick={handleSaveCampaign}
            variant="contained"
          >
            Save
          </LoadingButton>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
